package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// RecipeRepository is the Postgres-backed implementation of
// domain.RecipeRepository, backed by the recipes and recipe_ingredients
// tables (see internal/platform/db/migrations/0005_catalog.sql). Like
// ProductRepository, every method relies on row-level security (the
// tenant_isolation policy on both tables) rather than an application-level
// "tenant_id = $n" predicate, and every rehydrated Recipe's TenantID is
// set from scope.TenantID(), never re-parsed from the row.
type RecipeRepository struct {
	scope *db.Scope
}

// compile-time check: *RecipeRepository must satisfy domain.RecipeRepository.
var _ domain.RecipeRepository = (*RecipeRepository)(nil)

// NewRecipeRepository builds a RecipeRepository bound to scope -- see
// ProductRepository's NewProductRepository doc comment for why this
// constructor exists alongside UnitOfWork.Do/Repositories.Recipes().
func NewRecipeRepository(scope *db.Scope) *RecipeRepository {
	return &RecipeRepository{scope: scope}
}

// recipeNotFoundErr builds the *errs.Error a missing-row lookup returns.
func recipeNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "recipe_not_found", "recipe not found", cause)
}

const selectRecipeColumns = `id, name, description, version, created_at, updated_at`

const selectRecipeIngredientsSQL = `
	SELECT ingredient_id, (quantity * 1000)::bigint
	FROM recipe_ingredients
	WHERE recipe_id = $1
	ORDER BY ingredient_id ASC
`

// recipeRow is satisfied by pgx.Row, the shape FindByID's single-row query
// returns.
type recipeRow interface {
	Scan(dest ...any) error
}

// scanRecipeRow scans a single recipes row (in the exact column order
// selectRecipeColumns selects), loads its ingredient lines via a second
// query against the SAME scope, and rehydrates the whole thing into a
// *domain.Recipe via domain.NewRecipe plus the fields that constructor
// cannot know about (Version, CreatedAt, UpdatedAt).
func (r *RecipeRepository) scanRecipeRow(ctx context.Context, row recipeRow, tenantID tenant.TenantID) (*domain.Recipe, error) {
	var (
		idStr       string
		name        string
		description string
		version     int64
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := row.Scan(&idStr, &name, &description, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: parsing recipe id: %w", err)
	}

	ingredients, err := r.loadRecipeIngredients(ctx, idStr)
	if err != nil {
		return nil, err
	}

	recipe, err := domain.NewRecipe(id, tenantID, name, description, ingredients)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: rehydrating recipe: %w", err)
	}

	setUnexportedField(&recipe, "version", version)
	setUnexportedField(&recipe, "createdAt", createdAt.UTC())
	setUnexportedField(&recipe, "updatedAt", updatedAt.UTC())

	return &recipe, nil
}

// loadRecipeIngredients loads every recipe_ingredients row for
// recipeIDStr, rehydrating each into a domain.RecipeIngredient.
func (r *RecipeRepository) loadRecipeIngredients(ctx context.Context, recipeIDStr string) ([]domain.RecipeIngredient, error) {
	rows, err := r.scope.Query(ctx, selectRecipeIngredientsSQL, recipeIDStr)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: querying recipe ingredients: %w", err)
	}
	defer rows.Close()

	var ingredients []domain.RecipeIngredient
	for rows.Next() {
		var (
			ingredientIDStr string
			quantityRaw     int64
		)
		if err := rows.Scan(&ingredientIDStr, &quantityRaw); err != nil {
			return nil, fmt.Errorf("catalog/postgres: scanning recipe ingredient row: %w", err)
		}

		ingredientID, err := domain.ParseID(ingredientIDStr)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: parsing recipe ingredient id: %w", err)
		}
		quantity, err := domain.NewQuantity(quantityRaw)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: building recipe ingredient quantity: %w", err)
		}

		ingredient, err := domain.NewRecipeIngredient(ingredientID, quantity)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: rehydrating recipe ingredient: %w", err)
		}
		ingredients = append(ingredients, ingredient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog/postgres: iterating recipe ingredient rows: %w", err)
	}

	return ingredients, nil
}

// FindByID returns the Recipe identified by id, or a NotFound *errs.Error
// if none exists (or none is visible to the scope's tenant, per RLS).
func (r *RecipeRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Recipe, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectRecipeColumns+" FROM recipes WHERE id = $1", id.String())

	recipe, err := r.scanRecipeRow(ctx, row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, recipeNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: finding recipe by id: %w", err)
	}
	return recipe, nil
}

// Insert persists a brand-new Recipe, along with every one of its
// ingredient lines.
func (r *RecipeRepository) Insert(ctx context.Context, recipe *domain.Recipe) error {
	if _, err := r.scope.Exec(ctx, `
		INSERT INTO recipes (id, tenant_id, name, description, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		recipe.ID().String(), r.scope.TenantID().String(), recipe.Name(), recipe.Description(),
		recipe.Version(), recipe.CreatedAt(), recipe.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("catalog/postgres: inserting recipe: %w", err)
	}

	for _, ingredient := range recipe.Ingredients() {
		if _, err := r.scope.Exec(ctx, `
			INSERT INTO recipe_ingredients (tenant_id, recipe_id, ingredient_id, quantity)
			VALUES ($1, $2, $3, $4::numeric / 1000)
		`,
			r.scope.TenantID().String(), recipe.ID().String(), ingredient.IngredientID().String(),
			ingredient.Quantity().Thousandths(),
		); err != nil {
			return fmt.Errorf("catalog/postgres: inserting recipe ingredient: %w", err)
		}
	}

	return nil
}
