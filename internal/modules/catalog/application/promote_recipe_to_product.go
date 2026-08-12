package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// PromoteRecipeToProductInput carries everything PromoteRecipeToProduct
// needs to promote a single Recipe into a new sellable Product.
type PromoteRecipeToProductInput struct {
	RecipeID  domain.ID
	BasePrice domain.Money
}

// PromoteRecipeToProduct is the application-layer use case that promotes a
// Recipe into a brand-new Product, persisting the new Product and
// publishing a contracts.EventRecipePromoted domain event, all atomically
// within a single UnitOfWork.Do call.
//
// PromoteRecipeToProduct calls Recipe.PromoteToProduct, which enforces
// this module's core promotion invariant (a Recipe with zero ingredients
// cannot be promoted -- an *errs.Error of Kind errs.Invalid) and every
// Product construction invariant (non-empty name, strictly positive
// basePrice, no duplicate ingredients). Because that failure happens
// before any repository write is attempted, Do rolls back with nothing
// persisted. The Recipe itself is never mutated or re-persisted -- see
// Recipe.PromoteToProduct's own doc comment for why promotion is a pure,
// side-effect-free transformation in this phase.
func PromoteRecipeToProduct(ctx context.Context, uow domain.UnitOfWork, input PromoteRecipeToProductInput) (domain.Product, error) {
	var product domain.Product

	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		recipe, err := r.Recipes().FindByID(ctx, input.RecipeID)
		if err != nil {
			return err
		}

		promoted, err := recipe.PromoteToProduct(domain.NewID(), input.BasePrice)
		if err != nil {
			return err
		}

		if err := r.Products().Insert(ctx, &promoted); err != nil {
			return err
		}

		event, err := recipePromotedEvent(recipe, &promoted)
		if err != nil {
			return err
		}
		if err := r.Events().Publish(ctx, event); err != nil {
			return err
		}

		product = promoted
		return nil
	})
	if err != nil {
		return domain.Product{}, err
	}

	return product, nil
}

// recipePromotedEvent builds the EventRecipePromoted domain event linking
// recipe to the product it was just promoted into.
func recipePromotedEvent(recipe *domain.Recipe, product *domain.Product) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		recipe.TenantID(),
		contracts.EventRecipePromoted,
		recipe.ID().String(),
		product.CreatedAt(),
		contracts.RecipePromotedPayload{
			RecipeID:   recipe.ID().String(),
			ProductID:  product.ID().String(),
			OccurredAt: product.CreatedAt(),
		},
	)
}
