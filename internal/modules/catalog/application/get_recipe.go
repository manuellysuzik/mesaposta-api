package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// GetRecipe is the application-layer use case reading a single Recipe by
// id, via RecipeRepository.FindByID -- see GetProduct's doc comment for
// why this still goes through UnitOfWork.Do rather than a standalone
// repository.
func GetRecipe(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.Recipe, error) {
	var recipe *domain.Recipe
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Recipes().FindByID(ctx, id)
		if err != nil {
			return err
		}
		recipe = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return recipe, nil
}
