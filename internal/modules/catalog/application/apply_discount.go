package application

import (
	"context"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// Audit action/entity identifiers this use case records. Constants rather
// than inline literals for the same reason
// internal/modules/operations/application.ReceivePurchaseOrder's own
// audit constants are: the value of an audit trail depends on these
// identifiers being STABLE across time.
const (
	auditActionProductDiscountApplied = "product.discount_applied"
	auditEntityProduct                = "product"
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
//
// now and actorID feed the audit trail exactly as
// internal/modules/operations/application.ReceivePurchaseOrder's own now
// and actorID parameters do: now is when the discount was applied,
// actorID is a best-effort ("" when unknown, never a placeholder)
// identifier of who applied it, sourced by the HTTP layer from the
// authenticated access token's "sub" claim (see
// internal/platform/auth.ActorIDFrom). See docs/adr/0006-audit-log.md.
func ApplyDiscount(ctx context.Context, uow domain.UnitOfWork, input ApplyDiscountInput, now time.Time, actorID string) (domain.Product, error) {
	var updated domain.Product

	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		product, err := r.Products().FindByID(ctx, input.ProductID)
		if err != nil {
			return err
		}
		expectedVersion := product.Version()
		// Snapshot BEFORE product.ApplyDiscount mutates it in place -- see
		// operations.ReceivePurchaseOrder's identical snapshot-before-mutate
		// comment for why: reading these after the mutation would record a
		// "before" identical to the "after".
		priceBeforeDiscount := product.CurrentPrice()

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

		// The audit entry goes last, after every other write, on the same
		// principle operations.ReceivePurchaseOrder's own audit write
		// follows: because it runs inside this same Do call (the same
		// transaction), a failure anywhere -- including here -- rolls back
		// the discount AND its trail together.
		entry, err := audit.NewEntry(
			product.TenantID(),
			actorID,
			auditActionProductDiscountApplied,
			auditEntityProduct,
			product.ID().String(),
			now,
			map[string]any{
				"price_cents": priceBeforeDiscount.Cents(),
			},
			map[string]any{
				"price_cents":      product.CurrentPrice().Cents(),
				"discount_percent": hundredthsToPercent(*product.Discount()),
			},
		)
		if err != nil {
			return err
		}
		if err := r.Audit().Record(ctx, entry); err != nil {
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
