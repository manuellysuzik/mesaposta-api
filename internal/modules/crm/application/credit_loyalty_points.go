package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
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
func CreditLoyaltyPoints(ctx context.Context, uow domain.UnitOfWork, input CreditLoyaltyPointsInput) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		customer, err := r.Customers().FindByID(ctx, input.CustomerID)
		if err != nil {
			return err
		}
		expectedVersion := customer.Version()

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
		return r.Events().Publish(ctx, event)
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
