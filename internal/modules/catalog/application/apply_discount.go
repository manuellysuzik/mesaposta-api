package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// ApplyDiscountInput carries everything ApplyDiscount needs to apply a
// discount to a single Product.
type ApplyDiscountInput struct {
	ProductID domain.ID
	Discount  domain.DiscountPercent
}

// ApplyDiscount is the application-layer use case that applies a discount
// to a Product, updating its version and publishing a domain event, all
// atomically within a single UnitOfWork.Do call.
//
// ApplyDiscount calls Product.ApplyDiscount: if the discount would drive
// the product's final price to zero or below, ApplyDiscount returns an
// *errs.Error of Kind errs.Invalid -- and because that failure happens
// before ANY repository write is attempted, Do rolls back with nothing
// persisted. This is this module's core price invariant (a discounted
// price must stay strictly positive) proved once more through the
// application layer, mirroring
// internal/modules/operations/application.RegisterStockMovement's own
// "saida" branch for the identical shape of invariant.
func ApplyDiscount(ctx context.Context, uow domain.UnitOfWork, input ApplyDiscountInput) (domain.Product, error) {
	var updated domain.Product

	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		product, err := r.Products().FindByID(ctx, input.ProductID)
		if err != nil {
			return err
		}
		expectedVersion := product.Version()

		if err := product.ApplyDiscount(input.Discount); err != nil {
			return err
		}

		if err := r.Products().UpdateWithVersion(ctx, product, expectedVersion); err != nil {
			return err
		}

		event, err := productDiscountAppliedEvent(product)
		if err != nil {
			return err
		}
		if err := r.Events().Publish(ctx, event); err != nil {
			return err
		}

		updated = *product
		return nil
	})
	if err != nil {
		return domain.Product{}, err
	}

	return updated, nil
}

// productDiscountAppliedEvent builds the EventProductDiscountApplied
// domain event for product, carrying its CurrentPrice AFTER the discount
// was applied.
func productDiscountAppliedEvent(product *domain.Product) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		product.TenantID(),
		contracts.EventProductDiscountApplied,
		product.ID().String(),
		product.UpdatedAt(),
		contracts.ProductDiscountAppliedPayload{
			ProductID:       product.ID().String(),
			DiscountPercent: hundredthsToPercent(*product.Discount()),
			FinalPrice:      centsToCurrency(product.CurrentPrice()),
			OccurredAt:      product.UpdatedAt(),
		},
	)
}
