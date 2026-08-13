//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestTableRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)

	table := mustTable(t, ctx, repo, tenantID, "M01")

	found, err := repo.FindByID(ctx, table.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Code() != table.Code() {
		t.Errorf("found.Code() = %q, want %q", found.Code(), table.Code())
	}
	if found.IsActive() != table.IsActive() {
		t.Errorf("found.IsActive() = %v, want %v", found.IsActive(), table.IsActive())
	}
	if found.Version() != table.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), table.Version())
	}
}

func TestTableRepository_FindByCode_ReturnsRegardlessOfActiveState(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)

	table := mustTable(t, ctx, repo, tenantID, "M02")
	table.Deactivate()
	if err := repo.UpdateWithVersion(ctx, table, 1); err != nil {
		t.Fatalf("UpdateWithVersion: %v", err)
	}

	found, err := repo.FindByCode(ctx, "M02")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if found.IsActive() {
		t.Error("FindByCode returned IsActive()=true, want false (repository must not filter by active state)")
	}
}

func TestTableRepository_FindByCode_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)

	_, err = repo.FindByCode(ctx, "does-not-exist")
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestTableRepository_Insert_DuplicateCodeSameTenant_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000004")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)

	mustTable(t, ctx, repo, tenantID, "M03")

	dup, err := domain.NewTable(domain.NewID(), tenantID, "M03", "Mesa Duplicada")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	err = repo.Insert(ctx, &dup)
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestTableRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "40000000-0000-0000-0000-000000000005")
	tenantB := mustTenantID(t, "40000000-0000-0000-0000-000000000006")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewTableRepository(scopeA)
	table := mustTable(t, ctxA, repoA, tenantA, "M04")
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewTableRepository(scopeB)

	_, err = repoB.FindByID(ctxB, table.ID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound (RLS must hide cross-tenant rows)", errs.KindOf(err))
	}
}

func TestTableRepository_UpdateWithVersion_StaleVersion_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000007")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)
	table := mustTable(t, ctx, repo, tenantID, "M05")

	mutated := *table
	mutated.Deactivate()

	err = repo.UpdateWithVersion(ctx, &mutated, table.Version()+999)
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestTableRepository_FindAll_OrderedByCode(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000008")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTableRepository(scope)

	mustTable(t, ctx, repo, tenantID, "M20")
	mustTable(t, ctx, repo, tenantID, "M10")

	tables, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("len(tables) = %d, want 2", len(tables))
	}
	if tables[0].Code() != "M10" || tables[1].Code() != "M20" {
		t.Errorf("codes = [%q, %q], want [\"M10\", \"M20\"] (ascending)", tables[0].Code(), tables[1].Code())
	}
}
