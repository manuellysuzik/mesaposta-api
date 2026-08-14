package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// auditActionLoyaltyPointsRedeemed is this use case's audit action
// identifier -- see credit_loyalty_points.go's own constants for why this
// is a constant rather than an inline literal. auditEntityCustomer is
// shared with that file (same package, same entity type).
const auditActionLoyaltyPointsRedeemed = "loyalty_entry.redeemed"

// RedeemLoyaltyPointsInput carries everything RedeemLoyaltyPoints needs to
// redeem points from a single Customer's loyalty balance.
type RedeemLoyaltyPointsInput struct {
	CustomerID domain.ID
	Points     domain.LoyaltyPoints
	Reason     string
}

// RedeemLoyaltyPoints is the application-layer use case that redeems
// loyalty points from a Customer's balance and appends the matching
// LoyaltyEntry to their ledger, updating the customer's balance/version and
// publishing a domain event, all atomically within a single UnitOfWork.Do
// call.
//
// RedeemLoyaltyPoints calls Customer.RedeemPoints: if the requested points
// exceed the customer's current balance, RedeemPoints returns an
// *errs.Error of Kind errs.Conflict, RedeemLoyaltyPoints propagates it
// unchanged, and -- because that failure happens before ANY repository
// write is attempted -- Do rolls back with nothing persisted: neither the
// Customer's balance/version nor a LoyaltyEntry. This is the module's
// headline acceptance criterion (insufficient balance rejects the whole
// operation) proved once more, this time through the application layer
// rather than directly against the domain aggregate -- exactly mirroring
// internal/modules/operations/application.RegisterStockMovement's own
// "saida" branch for the identical shape of invariant.
//
// actorID identifies who requested the redemption, for the audit trail:
// best-effort ("" when unknown, never a placeholder), sourced by the HTTP
// layer from the authenticated access token's "sub" claim (see
// internal/platform/auth.ActorIDFrom). See docs/adr/0006-audit-log.md.
func RedeemLoyaltyPoints(ctx context.Context, uow domain.UnitOfWork, input RedeemLoyaltyPointsInput, actorID string) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		customer, err := r.Customers().FindByID(ctx, input.CustomerID)
		if err != nil {
			return err
		}
		expectedVersion := customer.Version()
		balanceBeforeRedeem := customer.LoyaltyPoints().Points()

		if err := customer.RedeemPoints(input.Points); err != nil {
			return err
		}

		entry, err := domain.NewLoyaltyEntry(
			domain.NewID(), customer.TenantID(), customer.ID(),
			domain.LoyaltyEntryTypeResgate, input.Points, input.Reason,
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

		event, err := loyaltyPointsRedeemedEvent(customer, entry)
		if err != nil {
			return err
		}
		if err := r.Events().Publish(ctx, event); err != nil {
			return err
		}

		// Audit entry recorded last -- see
		// CreditLoyaltyPoints' identical rationale.
		auditEntry, err := audit.NewEntry(
			customer.TenantID(),
			actorID,
			auditActionLoyaltyPointsRedeemed,
			auditEntityCustomer,
			customer.ID().String(),
			entry.CreatedAt(),
			map[string]any{"loyalty_points_balance": balanceBeforeRedeem},
			map[string]any{"loyalty_points_balance": customer.LoyaltyPoints().Points(), "points_redeemed": input.Points.Points(), "reason": input.Reason},
		)
		if err != nil {
			return err
		}
		return r.Audit().Record(ctx, auditEntry)
	})
}

// loyaltyPointsRedeemedEvent builds the EventLoyaltyPointsRedeemed domain
// event for entry, carrying customer's balance AFTER the redemption was
// applied.
func loyaltyPointsRedeemedEvent(customer *domain.Customer, entry domain.LoyaltyEntry) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		customer.TenantID(),
		contracts.EventLoyaltyPointsRedeemed,
		customer.ID().String(),
		entry.CreatedAt(),
		contracts.LoyaltyPointsRedeemedPayload{
			CustomerID:   customer.ID().String(),
			EntryID:      entry.ID().String(),
			Points:       entry.Points().Points(),
			BalanceAfter: customer.LoyaltyPoints().Points(),
			OccurredAt:   entry.CreatedAt(),
		},
	)
}
