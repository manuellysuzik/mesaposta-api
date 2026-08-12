package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustRecipeIngredient(t *testing.T, ingredientID domain.ID, quantity domain.Quantity) domain.RecipeIngredient {
	t.Helper()
	ing, err := domain.NewRecipeIngredient(ingredientID, quantity)
	if err != nil {
		t.Fatalf("NewRecipeIngredient: %v", err)
	}
	return ing
}

func TestNewRecipeIngredient_ZeroIngredientID_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewRecipeIngredient(domain.ID{}, mustQuantity(t, 1000))

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewRecipeIngredient_ZeroQuantity_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewRecipeIngredient(domain.NewID(), mustQuantity(t, 0))

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewRecipe_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	ingredients := []domain.RecipeIngredient{
		mustRecipeIngredient(t, domain.NewID(), mustQuantity(t, 500)),
	}

	// Act
	recipe, err := domain.NewRecipe(id, tenantID, "Molho especial", "Receita da casa", ingredients)

	// Assert
	if err != nil {
		t.Fatalf("NewRecipe returned unexpected error: %v", err)
	}
	if recipe.ID() != id {
		t.Errorf("recipe.ID() = %v, want %v", recipe.ID(), id)
	}
	if recipe.Name() != "Molho especial" {
		t.Errorf("recipe.Name() = %q, want %q", recipe.Name(), "Molho especial")
	}
	if len(recipe.Ingredients()) != 1 {
		t.Fatalf("len(recipe.Ingredients()) = %d, want 1", len(recipe.Ingredients()))
	}
	if recipe.Version() != 1 {
		t.Errorf("recipe.Version() = %d, want 1", recipe.Version())
	}
}

func TestNewRecipe_EmptyName_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "", "desc", nil)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

// TestNewRecipe_NoIngredients_Succeeds pins down the deliberate difference
// from NewProduct: a Recipe may exist as an in-progress draft with zero
// ingredients (see recipe.go's own doc comment) -- only PromoteToProduct
// enforces the "at least one ingredient" rule.
func TestNewRecipe_NoIngredients_Succeeds(t *testing.T) {
	// Act
	recipe, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "Rascunho", "desc", nil)

	// Assert
	if err != nil {
		t.Fatalf("NewRecipe with no ingredients returned unexpected error: %v", err)
	}
	if len(recipe.Ingredients()) != 0 {
		t.Errorf("len(recipe.Ingredients()) = %d, want 0", len(recipe.Ingredients()))
	}
}

func TestNewRecipe_DuplicateIngredient_ReturnsInvalidError(t *testing.T) {
	// Arrange
	dupID := domain.NewID()
	ingredients := []domain.RecipeIngredient{
		mustRecipeIngredient(t, dupID, mustQuantity(t, 500)),
		mustRecipeIngredient(t, dupID, mustQuantity(t, 200)),
	}

	// Act
	_, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "Molho especial", "desc", ingredients)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestRecipe_PromoteToProduct_WithIngredients_Succeeds(t *testing.T) {
	// Arrange
	ingredientID := domain.NewID()
	recipe, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "X-Burguer", "Hambúrguer artesanal",
		[]domain.RecipeIngredient{mustRecipeIngredient(t, ingredientID, mustQuantity(t, 200))})
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}
	productID := domain.NewID()

	// Act
	product, err := recipe.PromoteToProduct(productID, mustMoney(t, 2500))

	// Assert
	if err != nil {
		t.Fatalf("PromoteToProduct returned unexpected error: %v", err)
	}
	if product.ID() != productID {
		t.Errorf("product.ID() = %v, want %v", product.ID(), productID)
	}
	if product.TenantID() != recipe.TenantID() {
		t.Errorf("product.TenantID() = %v, want %v", product.TenantID(), recipe.TenantID())
	}
	if product.Name() != recipe.Name() {
		t.Errorf("product.Name() = %q, want %q", product.Name(), recipe.Name())
	}
	if product.BasePrice().Cents() != 2500 {
		t.Errorf("product.BasePrice().Cents() = %d, want 2500", product.BasePrice().Cents())
	}
	if len(product.Ingredients()) != 1 {
		t.Fatalf("len(product.Ingredients()) = %d, want 1", len(product.Ingredients()))
	}
	if product.Ingredients()[0].IngredientID() != ingredientID {
		t.Errorf("product.Ingredients()[0].IngredientID() = %v, want %v", product.Ingredients()[0].IngredientID(), ingredientID)
	}
	if product.Ingredients()[0].Quantity().Thousandths() != 200 {
		t.Errorf("product.Ingredients()[0].Quantity() = %v, want 200 thousandths", product.Ingredients()[0].Quantity())
	}
	if !product.IsActive() {
		t.Error("promoted product.IsActive() = false, want true")
	}
}

// TestRecipe_PromoteToProduct_NoIngredients_ReturnsInvalidError proves the
// task spec's core Recipe invariant: promotion requires at least one
// ingredient.
func TestRecipe_PromoteToProduct_NoIngredients_ReturnsInvalidError(t *testing.T) {
	// Arrange
	recipe, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "Rascunho", "desc", nil)
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}

	// Act
	_, err = recipe.PromoteToProduct(domain.NewID(), mustMoney(t, 1000))

	// Assert
	if err == nil {
		t.Fatal("PromoteToProduct with no ingredients returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestRecipe_PromoteToProduct_ZeroPrice_ReturnsInvalidError(t *testing.T) {
	// Arrange
	recipe, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "X-Burguer", "desc",
		[]domain.RecipeIngredient{mustRecipeIngredient(t, domain.NewID(), mustQuantity(t, 200))})
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}

	// Act
	_, err = recipe.PromoteToProduct(domain.NewID(), mustMoney(t, 0))

	// Assert: NewProduct's own price invariant is enforced through promotion too.
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestRecipe_PromoteToProduct_DoesNotMutateRecipe(t *testing.T) {
	// Arrange
	recipe, err := domain.NewRecipe(domain.NewID(), mustTenantID(t), "X-Burguer", "desc",
		[]domain.RecipeIngredient{mustRecipeIngredient(t, domain.NewID(), mustQuantity(t, 200))})
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}
	versionBefore := recipe.Version()

	// Act
	if _, err := recipe.PromoteToProduct(domain.NewID(), mustMoney(t, 1500)); err != nil {
		t.Fatalf("PromoteToProduct: %v", err)
	}

	// Assert
	if recipe.Version() != versionBefore {
		t.Errorf("recipe.Version() = %d after PromoteToProduct, want unchanged %d", recipe.Version(), versionBefore)
	}
}
