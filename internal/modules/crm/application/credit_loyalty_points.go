package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// Audit action/entity identifiers this use case records. Constants rather
// than inline literals for the same reason
// internal/modules/operations/application.ReceivePurchaseOrder's own audit
// constants are: the value of an audit trail depends on these identifiers
// being STABLE across time.
const (
	auditActionLoyaltyPointsCredited = "loyalty_entry.credited"
	auditEntityCustomer              = "customer"
)

// CreditLoyaltyPointsInput carries everything CreditLoyaltyPoints needs to
// credit points to a single Customer's loyalty balance.
type CreditLoyaltyPointsInput struct {
	CustomerID domain.ID
	Points     domain.LoyaltyPoints
	Reason     string
}

// CreditLoyaltyPoints is the application-layer use case that credits
// loyalty points to a Customer's balance and appends the matching
// LoyaltyEntry to their ledger, updating the customer's balance/version and
// publishing a domain event, all atomically within a single UnitOfWork.Do
// call -- see domain.Customer's own doc comment for why that atomicity
// (the whole Customer aggregate, including its nested ledger entry,
// persisted together) is load-bearing, not incidental.
//
// CreditLoyaltyPoints calls Customer.CreditPoints, which returns an
// *errs.Error of Kind errs.Invalid if the resulting balance would overflow
// -- and because that failure happens before any repository write is
// attempted, Do rolls back with nothing persisted.
//
// actorID identifies who requested the credit, for the audit trail: a
// best-effort ("" when unknown, never a placeholder) identifier sourced by
// the HTTP layer from the authenticated access token's "sub" claim (see
// internal/platform/auth.ActorIDFrom). See docs/adr/0006-audit-log.md.
func CreditLoyaltyPoints(ctx context.Context, uow domain.UnitOfWork, input CreditLoyaltyPointsInput, actorID string) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		customer, err := r.Customers().FindByID(ctx, input.CustomerID)
		if err != nil {
			return err
		}
		expectedVersion := customer.Version()
		balanceBeforeCredit := customer.LoyaltyPoints().Points()

		if err := customer.CreditPoints(input.Points); err != nil {
			return err
		}

		entry, err := domain.NewLoyaltyEntry(
			domain.NewID(), customer.TenantID(), customer.ID(),
			domain.LoyaltyEntryTypeCredito, input.Points, input.Reason,
		)
		if err != nil {
			return err
		}

		if err := r.Customers().UpdateWithVersion(ctx, customer, expectedVersion); err != nil {
			return err
		}
		if err := r.LoyaltyEntries().Insert(ctx, entry); err != nil {
			return err
		}

		event, err := loyaltyPointsCreditedEvent(customer, entry)
		if err != nil {
			return err
		}
		if err := r.Events().Publish(ctx, event); err != nil {
			return err
		}

		// The audit entry goes last, after every other write, on the same
		// principle operations.ReceivePurchaseOrder's own audit write
		// follows: because it runs inside this same Do call (the same
		// transaction), a failure anywhere -- including here -- rolls back
		// the credit AND its trail together. entry.CreatedAt() (not a
		// separately-threaded "now") is reused as the audit occurredAt time
		// since domain.NewLoyaltyEntry already stamped it.
		auditEntry, err := audit.NewEntry(
			customer.TenantID(),
			actorID,
			auditActionLoyaltyPointsCredited,
			auditEntityCustomer,
			customer.ID().String(),
			entry.CreatedAt(),
			map[string]any{"loyalty_points_balance": balanceBeforeCredit},
			map[string]any{"loyalty_points_balance": customer.LoyaltyPoints().Points(), "points_credited": input.Points.Points(), "reason": input.Reason},
		)
		if err != nil {
			return err
		}
		return r.Audit().Record(ctx, auditEntry)
	})
}

// loyaltyPointsCreditedEvent builds the EventLoyaltyPointsCredited domain
// event for entry, carrying customer's balance AFTER the credit was
// applied.
func loyaltyPointsCreditedEvent(customer *domain.Customer, entry domain.LoyaltyEntry) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		customer.TenantID(),
		contracts.EventLoyaltyPointsCredited,
		customer.ID().String(),
		entry.CreatedAt(),
		contracts.LoyaltyPointsCreditedPayload{
			CustomerID:   customer.ID().String(),
			EntryID:      entry.ID().String(),
			Points:       entry.Points().Points(),
			BalanceAfter: customer.LoyaltyPoints().Points(),
			OccurredAt:   entry.CreatedAt(),
		},
	)
}
