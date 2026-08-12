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

func TestProductRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewProductRepository(scope)

	ingredient, err := domain.NewProductIngredient(domain.NewID(), mustQuantity(t, 500))
	if err != nil {
		t.Fatalf("NewProductIngredient: %v", err)
	}
	product, err := domain.NewProduct(domain.NewID(), tenantID, "X-Burguer", "Hambúrguer artesanal", mustMoney(t, 1500),
		[]domain.ProductIngredient{ingredient})
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if err := product.ApplyDiscount(mustDiscountPercent(t, 1000)); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	if err := repo.Insert(ctx, &product); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := repo.FindByID(ctx, product.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != product.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), product.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.Name() != product.Name() {
		t.Errorf("found.Name() = %q, want %q", found.Name(), product.Name())
	}
	if found.BasePrice().Cents() != product.BasePrice().Cents() {
		t.Errorf("found.BasePrice() = %v, want %v", found.BasePrice(), product.BasePrice())
	}
	if found.Discount() == nil || found.Discount().Hundredths() != 1000 {
		t.Errorf("found.Discount() = %v, want 1000 hundredths", found.Discount())
	}
	if !found.IsActive() {
		t.Error("found.IsActive() = false, want true")
	}
	if len(found.Ingredients()) != 1 {
		t.Fatalf("len(found.Ingredients()) = %d, want 1", len(found.Ingredients()))
	}
	if found.Version() != product.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), product.Version())
	}
}

func TestProductRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewProductRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestProductRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "50000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "50000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewProductRepository(scopeA)
	product := mustProduct(t, ctxA, repoA, tenantA, "Produto A")
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewProductRepository(scopeB)

	_, err = repoB.FindByID(ctxB, product.ID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound (tenant B must not see tenant A's product)", errs.KindOf(err))
	}
}

func TestProductRepository_FindActive_OnlyReturnsActiveOwnTenantProducts(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "50000000-0000-0000-0000-000000000005")
	tenantB := mustTenantID(t, "50000000-0000-0000-0000-000000000006")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	defer scopeA.Rollback(ctxA)
	repoA := postgresadapter.NewProductRepository(scopeA)
	mustProduct(t, ctxA, repoA, tenantA, "Produto A1")
	mustProduct(t, ctxA, repoA, tenantA, "Produto A2")

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewProductRepository(scopeB)
	mustProduct(t, ctxB, repoB, tenantB, "Produto B1")

	active, err := repoA.FindActive(ctxA)
	if err != nil {
		t.Fatalf("FindActive: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("len(active) = %d, want 2 (only tenant A's products)", len(active))
	}
	if active[0].Name() != "Produto A1" || active[1].Name() != "Produto A2" {
		t.Errorf("active order = [%q, %q], want alphabetical [Produto A1, Produto A2]", active[0].Name(), active[1].Name())
	}
}

func TestProductRepository_UpdateWithVersion_StaleVersion_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000007")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewProductRepository(scope)
	product := mustProduct(t, ctx, repo, tenantID, "Produto Versao")

	mutated := *product
	if err := mutated.ApplyDiscount(mustDiscountPercent(t, 1000)); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}
	staleVersion := product.Version() + 999 // deliberately wrong

	err = repo.UpdateWithVersion(ctx, &mutated, staleVersion)
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestProductIngredients_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000008")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	repo := postgresadapter.NewProductRepository(setupScope)
	ingredient, err := domain.NewProductIngredient(domain.NewID(), mustQuantity(t, 500))
	if err != nil {
		t.Fatalf("NewProductIngredient: %v", err)
	}
	product, err := domain.NewProduct(domain.NewID(), tenantID, "Produto Ingrediente", "desc", mustMoney(t, 1000),
		[]domain.ProductIngredient{ingredient})
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if err := repo.Insert(ctx, &product); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE product_ingredients SET quantity = 999 WHERE product_id = $1", product.ID().String()); err == nil {
		t.Fatal("UPDATE product_ingredients: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
	if _, err := scope.Exec(ctx, "DELETE FROM product_ingredients WHERE product_id = $1", product.ID().String()); err == nil {
		t.Fatal("DELETE FROM product_ingredients: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}
