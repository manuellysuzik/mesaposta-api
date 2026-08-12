//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestStockItemRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "20000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewStockItemRepository(scope)

	supplierID := domain.NewID()
	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Farinha de trigo", domain.StockUnitKg,
		mustQuantity(t, 12345), mustQuantity(t, 500), mustMoney(t, 1099), &supplierID,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}

	if err := repo.Insert(ctx, &item); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := repo.FindByID(ctx, item.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != item.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), item.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.Name() != item.Name() {
		t.Errorf("found.Name() = %q, want %q", found.Name(), item.Name())
	}
	if found.Unit() != item.Unit() {
		t.Errorf("found.Unit() = %v, want %v", found.Unit(), item.Unit())
	}
	if found.Quantity().Thousandths() != item.Quantity().Thousandths() {
		t.Errorf("found.Quantity() = %v, want %v", found.Quantity(), item.Quantity())
	}
	if found.MinQuantity().Thousandths() != item.MinQuantity().Thousandths() {
		t.Errorf("found.MinQuantity() = %v, want %v", found.MinQuantity(), item.MinQuantity())
	}
	if found.CostPerUnit().Cents() != item.CostPerUnit().Cents() {
		t.Errorf("found.CostPerUnit() = %v, want %v", found.CostPerUnit(), item.CostPerUnit())
	}
	if found.SupplierID() == nil || *found.SupplierID() != supplierID {
		t.Errorf("found.SupplierID() = %v, want %v", found.SupplierID(), supplierID)
	}
	if found.Version() != item.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), item.Version())
	}
	if !found.UpdatedAt().Equal(item.UpdatedAt()) {
		t.Errorf("found.UpdatedAt() = %v, want %v", found.UpdatedAt(), item.UpdatedAt())
	}
}

func TestStockItemRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "20000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewStockItemRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if err == nil {
		t.Fatal("FindByID(unknown id): want an error, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestStockItemRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "20000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "20000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewStockItemRepository(scopeA)
	item := mustStockItem(t, ctxA, repoA, tenantA, 1000)
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewStockItemRepository(scopeB)

	_, err = repoB.FindByID(ctxB, item.ID())
	if err == nil {
		t.Fatal("FindByID: tenant B found tenant A's stock item, want it hidden by RLS")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestStockItemRepository_FindManyByIDs_CrossTenantIsolation(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "20000000-0000-0000-0000-000000000005")
	tenantB := mustTenantID(t, "20000000-0000-0000-0000-000000000006")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewStockItemRepository(scopeA)
	itemA1 := mustStockItem(t, ctxA, repoA, tenantA, 1000)
	itemA2 := mustStockItem(t, ctxA, repoA, tenantA, 2000)
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewStockItemRepository(scopeB)
	itemB := mustStockItem(t, ctxB, repoB, tenantB, 3000)

	found, err := repoB.FindManyByIDs(ctxB, []domain.ID{itemA1.ID(), itemA2.ID(), itemB.ID()})
	if err != nil {
		t.Fatalf("FindManyByIDs: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("FindManyByIDs returned %d item(s), want exactly 1 (only tenant B's own)", len(found))
	}
	if _, ok := found[itemB.ID()]; !ok {
		t.Error("FindManyByIDs did not return tenant B's own item")
	}
	if _, ok := found[itemA1.ID()]; ok {
		t.Error("FindManyByIDs returned tenant A's item to tenant B -- RLS isolation broken")
	}
}

func TestStockItemRepository_UpdateWithVersion_CrossTenant_AffectsZeroRows(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "20000000-0000-0000-0000-000000000007")
	tenantB := mustTenantID(t, "20000000-0000-0000-0000-000000000008")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewStockItemRepository(scopeA)
	item := mustStockItem(t, ctxA, repoA, tenantA, 1000)
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewStockItemRepository(scopeB)

	mutated := *item
	if err := mutated.Receive(mustQuantity(t, 500)); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	err = repoB.UpdateWithVersion(ctxB, &mutated, item.Version())
	if err == nil {
		t.Fatal("UpdateWithVersion: tenant B updated tenant A's stock item, want it rejected")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict (RowsAffected=0)", errs.KindOf(err))
	}
}

// TestStockItemRepository_UpdateWithVersion_ConcurrentSameVersion_ExactlyOneWinner
// is T5.3's headline optimistic-concurrency test: two concurrent
// UpdateWithVersion calls against the SAME row, both presenting the SAME
// (correct, pre-mutation) expectedVersion, must produce exactly one
// success and one errs.Conflict -- never two successes (which would mean
// one write silently clobbered the other) and never zero successes.
func TestStockItemRepository_UpdateWithVersion_ConcurrentSameVersion_ExactlyOneWinner(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "20000000-0000-0000-0000-000000000009")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	setupRepo := postgresadapter.NewStockItemRepository(setupScope)
	item := mustStockItem(t, ctx, setupRepo, tenantID, 1000)
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	addQty := mustQuantity(t, 10)

	const attempts = 5
	var wg sync.WaitGroup
	results := make([]error, attempts)
	start := make(chan struct{})

	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			<-start

			scope, err := pool.Scope(ctx)
			if err != nil {
				results[i] = err
				return
			}
			defer scope.Rollback(ctx)
			repo := postgresadapter.NewStockItemRepository(scope)

			mutated := *item
			if err := mutated.Receive(addQty); err != nil {
				results[i] = err
				return
			}
			if err := repo.UpdateWithVersion(ctx, &mutated, item.Version()); err != nil {
				results[i] = err
				return
			}
			results[i] = scope.Commit(ctx)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	conflicts := 0
	for _, err := range results {
		switch {
		case err == nil:
			winners++
		case errs.IsKind(err, errs.Conflict):
			conflicts++
		default:
			t.Errorf("unexpected error kind: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d winner(s) out of %d concurrent UpdateWithVersion calls sharing the same version, want exactly 1 (results: %v)", winners, attempts, results)
	}
	if conflicts != attempts-1 {
		t.Fatalf("got %d conflict(s), want %d", conflicts, attempts-1)
	}

	verifyScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (verify): %v", err)
	}
	defer verifyScope.Rollback(ctx)
	verifyRepo := postgresadapter.NewStockItemRepository(verifyScope)
	final, err := verifyRepo.FindByID(ctx, item.ID())
	if err != nil {
		t.Fatalf("FindByID (verify): %v", err)
	}
	if final.Version() != item.Version()+1 {
		t.Errorf("final version = %d, want %d (exactly one successful update)", final.Version(), item.Version()+1)
	}
	if final.Quantity().Thousandths() != item.Quantity().Thousandths()+10 {
		t.Errorf("final quantity = %v, want %d (exactly one update applied)", final.Quantity(), item.Quantity().Thousandths()+10)
	}
}

func TestStockItemRepository_Insert_NegativeQuantity_RejectedByDatabaseCheck(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "20000000-0000-0000-0000-00000000000a")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	// domain.NewQuantity already forbids a negative value, so the only way
	// to exercise the database's own CHECK (quantity >= 0) constraint is to
	// bypass the repository/domain layer and issue the INSERT directly.
	_, err = scope.Exec(ctx, `
		INSERT INTO stock_items (id, tenant_id, name, unit, quantity, min_quantity, cost_per_unit, version, updated_at)
		VALUES ($1, $2, 'Item invalido', 'kg', -1, 0, 1, 1, now())
	`, domain.NewID().String(), tenantID.String())
	if err == nil {
		t.Fatal("INSERT with negative quantity: want the database to reject it via CHECK (quantity >= 0), got nil error")
	}
}

func TestStockMovements_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "20000000-0000-0000-0000-00000000000b")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	itemRepo := postgresadapter.NewStockItemRepository(setupScope)
	item := mustStockItem(t, ctx, itemRepo, tenantID, 1000)

	movement, err := domain.NewStockMovement(domain.NewID(), tenantID, item.ID(), domain.StockMovementTypeEntrada, mustQuantity(t, 1000), "recebimento")
	if err != nil {
		t.Fatalf("NewStockMovement: %v", err)
	}
	movementRepo := postgresadapter.NewStockMovementRepository(setupScope)
	if err := movementRepo.Insert(ctx, movement); err != nil {
		t.Fatalf("Insert(movement): %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE stock_movements SET reason = 'tampered' WHERE id = $1", movement.ID().String()); err == nil {
		t.Fatal("UPDATE stock_movements: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}

	if _, err := scope.Exec(ctx, "DELETE FROM stock_movements WHERE id = $1", movement.ID().String()); err == nil {
		t.Fatal("DELETE FROM stock_movements: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}
