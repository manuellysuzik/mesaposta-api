//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestPurchaseOrderRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	itemRepo := postgresadapter.NewStockItemRepository(scope)
	item1 := mustStockItem(t, ctx, itemRepo, tenantID, 0)
	item2 := mustStockItem(t, ctx, itemRepo, tenantID, 0)

	poItem1, err := domain.NewPurchaseOrderItem(item1.ID(), mustQuantity(t, 2000), mustMoney(t, 500))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem(1): %v", err)
	}
	poItem2, err := domain.NewPurchaseOrderItem(item2.ID(), mustQuantity(t, 1000), mustMoney(t, 300))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem(2): %v", err)
	}
	supplierID := domain.NewID()
	order, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, supplierID, []domain.PurchaseOrderItem{poItem1, poItem2})
	if err != nil {
		t.Fatalf("NewPurchaseOrder: %v", err)
	}

	orderRepo := postgresadapter.NewPurchaseOrderRepository(scope)
	if err := orderRepo.Insert(ctx, &order); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := orderRepo.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != order.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), order.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.SupplierID() != supplierID {
		t.Errorf("found.SupplierID() = %v, want %v", found.SupplierID(), supplierID)
	}
	if found.Status() != domain.PurchaseOrderStatusPendente {
		t.Errorf("found.Status() = %v, want %v", found.Status(), domain.PurchaseOrderStatusPendente)
	}
	if found.Version() != order.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), order.Version())
	}
	if found.ReceivedAt() != nil {
		t.Errorf("found.ReceivedAt() = %v, want nil", found.ReceivedAt())
	}
	items := found.Items()
	if len(items) != 2 {
		t.Fatalf("len(found.Items()) = %d, want 2", len(items))
	}

	byStockItem := map[domain.ID]domain.PurchaseOrderItem{}
	for _, it := range items {
		byStockItem[it.StockItemID()] = it
	}
	got1, ok := byStockItem[item1.ID()]
	if !ok {
		t.Fatal("found order is missing item1")
	}
	if got1.Quantity().Thousandths() != 2000 {
		t.Errorf("item1 quantity = %v, want 2000", got1.Quantity())
	}
	if got1.CostPerUnit().Cents() != 500 {
		t.Errorf("item1 cost = %v, want 500", got1.CostPerUnit())
	}
}

func TestPurchaseOrderRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewPurchaseOrderRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if err == nil {
		t.Fatal("FindByID(unknown): want an error, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestPurchaseOrderRepository_UpdateWithVersion_MarksReceived(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	itemRepo := postgresadapter.NewStockItemRepository(setupScope)
	item := mustStockItem(t, ctx, itemRepo, tenantID, 0)
	orderRepo := postgresadapter.NewPurchaseOrderRepository(setupScope)
	order := mustPurchaseOrder(t, ctx, orderRepo, tenantID, item.ID(), 1000, 200)
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewPurchaseOrderRepository(scope)

	locked, err := repo.FindByIDForUpdate(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByIDForUpdate: %v", err)
	}
	if len(locked.Items()) != 1 {
		t.Fatalf("len(locked.Items()) = %d, want 1 (loaded in the same round trip)", len(locked.Items()))
	}

	now := time.Now().UTC()
	if err := locked.MarkAsReceived(now); err != nil {
		t.Fatalf("MarkAsReceived: %v", err)
	}
	if err := repo.UpdateWithVersion(ctx, locked, order.Version()); err != nil {
		t.Fatalf("UpdateWithVersion: %v", err)
	}
	if err := scope.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	verifyScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (verify): %v", err)
	}
	defer verifyScope.Rollback(ctx)
	verifyRepo := postgresadapter.NewPurchaseOrderRepository(verifyScope)
	found, err := verifyRepo.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID (verify): %v", err)
	}
	if found.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("found.Status() = %v, want %v", found.Status(), domain.PurchaseOrderStatusRecebido)
	}
	if found.ReceivedAt() == nil {
		t.Fatal("found.ReceivedAt() = nil, want non-nil")
	}
	if found.Version() != order.Version()+1 {
		t.Errorf("found.Version() = %d, want %d", found.Version(), order.Version()+1)
	}
}
