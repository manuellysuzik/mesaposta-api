package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateProductInput carries everything CreateProduct needs to build a
// brand-new Product.
type CreateProductInput struct {
	TenantID    tenant.TenantID
	Name        string
	Description string
	BasePrice   domain.Money
	Ingredients []domain.ProductIngredient
}

// CreateProduct is the application-layer use case that creates a new
// Product. It builds the aggregate via domain.NewProduct -- which performs
// all field validation, including the "price must be strictly positive"
// and "no duplicate ingredient" invariants -- and, only if that succeeds,
// persists it and publishes a contracts.EventProductCreated domain event,
// all atomically within a single UnitOfWork.Do call.
func CreateProduct(ctx context.Context, uow domain.UnitOfWork, input CreateProductInput) (domain.Product, error) {
	product, err := domain.NewProduct(
		domain.NewID(), input.TenantID, input.Name, input.Description, input.BasePrice, input.Ingredients,
	)
	if err != nil {
		return domain.Product{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Products().Insert(ctx, &product); err != nil {
			return err
		}

		event, err := productCreatedEvent(&product)
		if err != nil {
			return err
		}
		return r.Events().Publish(ctx, event)
	})
	if err != nil {
		return domain.Product{}, err
	}

	return product, nil
}

// productCreatedEvent builds the EventProductCreated domain event for a
// freshly created product.
func productCreatedEvent(product *domain.Product) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		product.TenantID(),
		contracts.EventProductCreated,
		product.ID().String(),
		product.CreatedAt(),
		contracts.ProductCreatedPayload{
			ProductID:  product.ID().String(),
			Name:       product.Name(),
			BasePrice:  centsToCurrency(product.BasePrice()),
			OccurredAt: product.CreatedAt(),
		},
	)
}
