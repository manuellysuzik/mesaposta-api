package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestPromoteRecipeToProduct_WithIngredients_CreatesProductAndPublishesEvent(t *testing.T) {
	// Arrange
	ingredientID := domain.NewID()
	ingredient := mustRecipeIngredientForTest(t, ingredientID, 200)
	recipe := mustFakeRecipe(t, []domain.RecipeIngredient{ingredient})
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)

	// Act
	product, err := application.PromoteRecipeToProduct(context.Background(), uow, application.PromoteRecipeToProductInput{
		RecipeID:  recipe.ID(),
		BasePrice: mustFakeMoney(t, 2500),
	})

	// Assert
	if err != nil {
		t.Fatalf("PromoteRecipeToProduct: unexpected error: %v", err)
	}
	if product.Name() != recipe.Name() {
		t.Errorf("product.Name() = %q, want %q", product.Name(), recipe.Name())
	}
	if len(product.Ingredients()) != 1 {
		t.Fatalf("len(product.Ingredients()) = %d, want 1", len(product.Ingredients()))
	}

	snapshot := uow.snapshot()
	if _, ok := snapshot.products[product.ID()]; !ok {
		t.Fatal("promoted product was not persisted")
	}
	if len(snapshot.events) != 1 {
		t.Fatalf("events published = %d, want 1", len(snapshot.events))
	}
}

func TestPromoteRecipeToProduct_RecipeNotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.PromoteRecipeToProduct(context.Background(), uow, application.PromoteRecipeToProductInput{
		RecipeID:  domain.NewID(),
		BasePrice: mustFakeMoney(t, 1000),
	})

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}

// TestPromoteRecipeToProduct_NoIngredients_PersistsNothing proves this
// module's core Recipe invariant through the application layer: a Recipe
// with no ingredients cannot be promoted, and nothing is persisted when it
// is rejected.
func TestPromoteRecipeToProduct_NoIngredients_PersistsNothing(t *testing.T) {
	// Arrange
	recipe := mustFakeRecipe(t, nil)
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)

	// Act
	_, err := application.PromoteRecipeToProduct(context.Background(), uow, application.PromoteRecipeToProductInput{
		RecipeID:  recipe.ID(),
		BasePrice: mustFakeMoney(t, 1000),
	})

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	snapshot := uow.snapshot()
	if len(snapshot.products) != 0 {
		t.Errorf("products = %v, want empty", snapshot.products)
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty", snapshot.events)
	}
}

func mustRecipeIngredientForTest(t *testing.T, ingredientID domain.ID, thousandths int64) domain.RecipeIngredient {
	t.Helper()
	ing, err := domain.NewRecipeIngredient(ingredientID, mustFakeQuantity(t, thousandths))
	if err != nil {
		t.Fatalf("NewRecipeIngredient: %v", err)
	}
	return ing
}
