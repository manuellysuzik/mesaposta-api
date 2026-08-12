//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestRecipeRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewRecipeRepository(scope)

	recipe := mustRecipe(t, ctx, repo, tenantID, "Molho especial")

	found, err := repo.FindByID(ctx, recipe.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != recipe.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), recipe.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.Name() != recipe.Name() {
		t.Errorf("found.Name() = %q, want %q", found.Name(), recipe.Name())
	}
	if len(found.Ingredients()) != 1 {
		t.Fatalf("len(found.Ingredients()) = %d, want 1", len(found.Ingredients()))
	}
	if found.Version() != recipe.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), recipe.Version())
	}
}

func TestRecipeRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewRecipeRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestRecipeRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "60000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "60000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewRecipeRepository(scopeA)
	recipe := mustRecipe(t, ctxA, repoA, tenantA, "Receita A")
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewRecipeRepository(scopeB)

	_, err = repoB.FindByID(ctxB, recipe.ID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound (tenant B must not see tenant A's recipe)", errs.KindOf(err))
	}
}

func TestRecipeIngredients_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000005")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	repo := postgresadapter.NewRecipeRepository(setupScope)
	recipe := mustRecipe(t, ctx, repo, tenantID, "Receita Ledger")
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE recipe_ingredients SET quantity = 999 WHERE recipe_id = $1", recipe.ID().String()); err == nil {
		t.Fatal("UPDATE recipe_ingredients: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
	if _, err := scope.Exec(ctx, "DELETE FROM recipe_ingredients WHERE recipe_id = $1", recipe.ID().String()); err == nil {
		t.Fatal("DELETE FROM recipe_ingredients: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}
