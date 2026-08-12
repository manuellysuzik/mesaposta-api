package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

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
func RedeemLoyaltyPoints(ctx context.Context, uow domain.UnitOfWork, input RedeemLoyaltyPointsInput) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		customer, err := r.Customers().FindByID(ctx, input.CustomerID)
		if err != nil {
			return err
		}
		expectedVersion := customer.Version()

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
		return r.Events().Publish(ctx, event)
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
