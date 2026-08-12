//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestReceivePurchaseOrder_ConcurrentReceipts_ExactlyOneWinner(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	itemRepo := postgresadapter.NewStockItemRepository(setupScope)
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
	order, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, domain.NewID(), []domain.PurchaseOrderItem{poItem1, poItem2})
	if err != nil {
		t.Fatalf("NewPurchaseOrder: %v", err)
	}
	orderRepo := postgresadapter.NewPurchaseOrderRepository(setupScope)
	if err := orderRepo.Insert(ctx, &order); err != nil {
		t.Fatalf("Insert (order): %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	// The explicit barrier: both goroutines block on the same channel and
	// are released together, maximizing the chance that both are genuinely
	// mid-flight (one holding the FindByIDForUpdate row lock, the other
	// blocked waiting to acquire it) rather than trivially running one
	// after the other.
	const attempts = 2
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]error, attempts)

	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = application.ReceivePurchaseOrder(ctx, uow, order.ID(), time.Now().UTC())
		}(i)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errs.IsKind(err, errs.Conflict):
			conflicts++
		default:
			t.Errorf("unexpected error kind (want nil or errs.Conflict): %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("got %d successful concurrent ReceivePurchaseOrder calls, want exactly 1 (results: %v)", successes, results)
	}
	if conflicts != attempts-1 {
		t.Fatalf("got %d errs.Conflict result(s), want %d (results: %v)", conflicts, attempts-1, results)
	}

	// Verify: exactly N stock_movements (N = number of order items = 2, not
	// 2N), exactly 1 transaction referencing this order, and each stock
	// item's quantity was summed in exactly once.
	verifyScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (verify): %v", err)
	}
	defer verifyScope.Rollback(ctx)

	var movementCount int
	if err := verifyScope.QueryRow(ctx, `
		SELECT count(*) FROM stock_movements WHERE stock_item_id IN ($1, $2)
	`, item1.ID().String(), item2.ID().String()).Scan(&movementCount); err != nil {
		t.Fatalf("counting stock_movements: %v", err)
	}
	if movementCount != 2 {
		t.Errorf("stock_movements count = %d, want 2 (N = number of order items, not 2N)", movementCount)
	}

	var transactionCount int
	if err := verifyScope.QueryRow(ctx, `
		SELECT count(*) FROM transactions WHERE reference_id = $1
	`, order.ID().String()).Scan(&transactionCount); err != nil {
		t.Fatalf("counting transactions: %v", err)
	}
	if transactionCount != 1 {
		t.Errorf("transactions count (reference_id = order id) = %d, want exactly 1", transactionCount)
	}

	verifyItemRepo := postgresadapter.NewStockItemRepository(verifyScope)
	found1, err := verifyItemRepo.FindByID(ctx, item1.ID())
	if err != nil {
		t.Fatalf("FindByID (item1): %v", err)
	}
	if found1.Quantity().Thousandths() != 2000 {
		t.Errorf("item1 final quantity = %v, want 2000 (received exactly once, not doubled)", found1.Quantity())
	}
	found2, err := verifyItemRepo.FindByID(ctx, item2.ID())
	if err != nil {
		t.Fatalf("FindByID (item2): %v", err)
	}
	if found2.Quantity().Thousandths() != 1000 {
		t.Errorf("item2 final quantity = %v, want 1000 (received exactly once, not doubled)", found2.Quantity())
	}

	verifyOrderRepo := postgresadapter.NewPurchaseOrderRepository(verifyScope)
	foundOrder, err := verifyOrderRepo.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID (order): %v", err)
	}
	if foundOrder.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("order status = %v, want %v", foundOrder.Status(), domain.PurchaseOrderStatusRecebido)
	}
}
