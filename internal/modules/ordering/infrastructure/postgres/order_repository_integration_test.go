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

func TestOrderRepository_InsertAndFindByID_RoundTrip_IncludingItems(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	tableRepo := postgresadapter.NewTableRepository(scope)
	orderRepo := postgresadapter.NewOrderRepository(scope)

	table := mustTable(t, ctx, tableRepo, tenantID, "M01")
	order := mustOrder(t, ctx, orderRepo, tenantID, table.ID())

	found, err := orderRepo.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.TableID() != table.ID() {
		t.Errorf("found.TableID() = %v, want %v", found.TableID(), table.ID())
	}
	if found.Status() != domain.OrderStatusPendenteConfirmacao {
		t.Errorf("found.Status() = %q, want %q", found.Status(), domain.OrderStatusPendenteConfirmacao)
	}
	items := found.Items()
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ProductID() != "catalog-product-1" || items[0].Quantity().Count() != 2 || items[0].UnitPrice().Cents() != 1500 {
		t.Errorf("items[0] = %+v, want catalog-product-1/2/1500", items[0])
	}
}

func TestOrderRepository_UpdateWithVersion_PersistsStatusTransition(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	tableRepo := postgresadapter.NewTableRepository(scope)
	orderRepo := postgresadapter.NewOrderRepository(scope)

	table := mustTable(t, ctx, tableRepo, tenantID, "M02")
	order := mustOrder(t, ctx, orderRepo, tenantID, table.ID())

	mutated := *order
	if err := mutated.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := orderRepo.UpdateWithVersion(ctx, &mutated, order.Version()); err != nil {
		t.Fatalf("UpdateWithVersion: %v", err)
	}

	found, err := orderRepo.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Status() != domain.OrderStatusConfirmado {
		t.Errorf("found.Status() = %q, want %q", found.Status(), domain.OrderStatusConfirmado)
	}
	if found.Version() != order.Version()+1 {
		t.Errorf("found.Version() = %d, want %d", found.Version(), order.Version()+1)
	}
}

func TestOrderRepository_UpdateWithVersion_StaleVersion_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	tableRepo := postgresadapter.NewTableRepository(scope)
	orderRepo := postgresadapter.NewOrderRepository(scope)

	table := mustTable(t, ctx, tableRepo, tenantID, "M03")
	order := mustOrder(t, ctx, orderRepo, tenantID, table.ID())

	mutated := *order
	if err := mutated.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	err = orderRepo.UpdateWithVersion(ctx, &mutated, order.Version()+999)
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestOrderRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "50000000-0000-0000-0000-000000000004")
	tenantB := mustTenantID(t, "50000000-0000-0000-0000-000000000005")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	tableRepoA := postgresadapter.NewTableRepository(scopeA)
	orderRepoA := postgresadapter.NewOrderRepository(scopeA)
	tableA := mustTable(t, ctxA, tableRepoA, tenantA, "M04")
	orderA := mustOrder(t, ctxA, orderRepoA, tenantA, tableA.ID())
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	orderRepoB := postgresadapter.NewOrderRepository(scopeB)

	_, err = orderRepoB.FindByID(ctxB, orderA.ID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound (RLS must hide cross-tenant rows)", errs.KindOf(err))
	}
}

func TestOrderRepository_FindAll_OrderedByCreatedAtDescending(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000006")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	tableRepo := postgresadapter.NewTableRepository(scope)
	orderRepo := postgresadapter.NewOrderRepository(scope)

	table := mustTable(t, ctx, tableRepo, tenantID, "M06")
	first := mustOrder(t, ctx, orderRepo, tenantID, table.ID())
	second := mustOrder(t, ctx, orderRepo, tenantID, table.ID())

	orders, err := orderRepo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("len(orders) = %d, want 2", len(orders))
	}
	if orders[0].ID() != second.ID() || orders[1].ID() != first.ID() {
		t.Error("FindAll did not return orders ordered descending by created_at")
	}
}
