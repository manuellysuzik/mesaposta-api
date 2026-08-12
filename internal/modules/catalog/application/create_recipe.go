package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateRecipeInput carries everything CreateRecipe needs to build a
// brand-new Recipe.
type CreateRecipeInput struct {
	TenantID    tenant.TenantID
	Name        string
	Description string
	Ingredients []domain.RecipeIngredient
}

// CreateRecipe is the application-layer use case that creates a new
// Recipe. It builds the aggregate via domain.NewRecipe -- which performs
// all field validation -- and, only if that succeeds, persists it via
// Recipes().Insert within a single UnitOfWork.Do call.
//
// Unlike CreateProduct, CreateRecipe does not publish a domain event: this
// module's event set (see contracts/events.go) only covers ProductCreated,
// ProductDiscountApplied and RecipePromoted -- a Recipe becoming visible to
// other parts of the system happens at promotion time (PromoteRecipeToProduct),
// not at creation time, since a Recipe may still be an in-progress draft
// (see domain.NewRecipe's own doc comment).
func CreateRecipe(ctx context.Context, uow domain.UnitOfWork, input CreateRecipeInput) (domain.Recipe, error) {
	recipe, err := domain.NewRecipe(domain.NewID(), input.TenantID, input.Name, input.Description, input.Ingredients)
	if err != nil {
		return domain.Recipe{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Recipes().Insert(ctx, &recipe)
	})
	if err != nil {
		return domain.Recipe{}, err
	}

	return recipe, nil
}
