package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// receiveFixture bundles everything a ReceivePurchaseOrder test needs: one
// pending PurchaseOrder referencing two StockItems, and the fakeUnitOfWork
// it (and only it) is seeded into.
type receiveFixture struct {
	uow        *fakeUnitOfWork
	order      *domain.PurchaseOrder
	stockItem1 *domain.StockItem
	stockItem2 *domain.StockItem
}

func newReceiveFixture(t *testing.T) receiveFixture {
	t.Helper()
	tenantID := mustFakeUOWTenantID(t)

	item1, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Farinha de trigo", domain.StockUnitKg,
		mustFakeQuantity(t, 0), mustFakeQuantity(t, 0), mustMoneyFor(t, 500), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem(1): %v", err)
	}
	item2, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Acucar", domain.StockUnitKg,
		mustFakeQuantity(t, 0), mustFakeQuantity(t, 0), mustMoneyFor(t, 300), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem(2): %v", err)
	}

	poItem1, err := domain.NewPurchaseOrderItem(item1.ID(), mustFakeQuantity(t, 2000), mustMoneyFor(t, 500))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem(1): %v", err)
	}
	poItem2, err := domain.NewPurchaseOrderItem(item2.ID(), mustFakeQuantity(t, 1000), mustMoneyFor(t, 300))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem(2): %v", err)
	}

	order, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, domain.NewID(), []domain.PurchaseOrderItem{poItem1, poItem2})
	if err != nil {
		t.Fatalf("NewPurchaseOrder: %v", err)
	}

	state := newFakeState()
	state.stockItems[item1.ID()] = &item1
	state.stockItems[item2.ID()] = &item2
	state.purchaseOrders[order.ID()] = &order

	return receiveFixture{
		uow:        newFakeUnitOfWork(state),
		order:      &order,
		stockItem1: &item1,
		stockItem2: &item2,
	}
}

func mustMoneyFor(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

func TestReceivePurchaseOrder_HappyPath_PersistsEverythingAtomically(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()

	// Act
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now)

	// Assert
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder: unexpected error: %v", err)
	}

	snapshot := f.uow.snapshot()

	persistedOrder, ok := snapshot.purchaseOrders[f.order.ID()]
	if !ok {
		t.Fatal("purchase order was not persisted")
	}
	if persistedOrder.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("persisted order status = %v, want %v", persistedOrder.Status(), domain.PurchaseOrderStatusRecebido)
	}

	item1, ok := snapshot.stockItems[f.stockItem1.ID()]
	if !ok {
		t.Fatal("stock item 1 was not persisted")
	}
	if item1.Quantity().Thousandths() != 2000 {
		t.Errorf("stock item 1 quantity = %d, want 2000", item1.Quantity().Thousandths())
	}

	item2, ok := snapshot.stockItems[f.stockItem2.ID()]
	if !ok {
		t.Fatal("stock item 2 was not persisted")
	}
	if item2.Quantity().Thousandths() != 1000 {
		t.Errorf("stock item 2 quantity = %d, want 1000", item2.Quantity().Thousandths())
	}

	if len(snapshot.stockMovements) != 2 {
		t.Fatalf("len(stockMovements) = %d, want 2", len(snapshot.stockMovements))
	}
	if len(snapshot.transactions) != 1 {
		t.Fatalf("len(transactions) = %d, want 1", len(snapshot.transactions))
	}

	// Events: one per stock item movement, one purchase order received, one
	// transaction recorded -- 4 total.
	if len(snapshot.events) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(snapshot.events))
	}
}

func TestReceivePurchaseOrder_ConflictOnOrderUpdate_PersistsNothing(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()
	f.uow.errors.purchaseOrderUpdateWithVersion = errs.New(errs.Conflict, "purchase_order_version_conflict",
		"purchase order was concurrently modified", nil)

	// Act
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now)

	// Assert
	if err == nil {
		t.Fatal("ReceivePurchaseOrder: expected an error, got nil")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	snapshot := f.uow.snapshot()
	persistedOrder := snapshot.purchaseOrders[f.order.ID()]
	if persistedOrder.Status() != domain.PurchaseOrderStatusPendente {
		t.Errorf("persisted order status = %v, want unchanged %v", persistedOrder.Status(), domain.PurchaseOrderStatusPendente)
	}
	if snapshot.stockItems[f.stockItem1.ID()].Quantity().Thousandths() != 0 {
		t.Error("stock item 1 was mutated despite the purchase order update conflict")
	}
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
	if len(snapshot.transactions) != 0 {
		t.Errorf("transactions = %v, want empty", snapshot.transactions)
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty", snapshot.events)
	}
}

func TestReceivePurchaseOrder_PersistsPurchaseOrderBeforeAnyOtherWrite(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()

	// Act
	if err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now); err != nil {
		t.Fatalf("ReceivePurchaseOrder: unexpected error: %v", err)
	}

	// Assert
	trail := f.uow.Trail()
	if len(trail) == 0 {
		t.Fatal("Trail() is empty, want at least one recorded write")
	}
	if trail[0] != "PurchaseOrders.UpdateWithVersion" {
		t.Errorf("trail[0] = %q, want %q (full trail: %v)", trail[0], "PurchaseOrders.UpdateWithVersion", trail)
	}
	for _, name := range trail[1:] {
		if name == "PurchaseOrders.Insert" || name == "PurchaseOrders.UpdateWithVersion" {
			t.Errorf("purchase order write %q found after index 0 in trail %v", name, trail)
		}
	}
}

func TestReceivePurchaseOrder_RepositoryErrorKindPropagatesThroughDo(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()
	f.uow.errors.stockMovementInsert = errs.New(errs.Internal, "stock_movement_insert_failed",
		"simulated infrastructure failure", errors.New("connection reset"))

	// Act
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now)

	// Assert
	if err == nil {
		t.Fatal("ReceivePurchaseOrder: expected an error, got nil")
	}
	if !errs.IsKind(err, errs.Internal) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Internal)
	}

	snapshot := f.uow.snapshot()
	if snapshot.purchaseOrders[f.order.ID()].Status() != domain.PurchaseOrderStatusPendente {
		t.Error("purchase order was persisted despite downstream stock movement insert failure")
	}
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
}

func TestReceivePurchaseOrder_AlreadyReceivedOrder_PersistsNothing(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()

	// Pre-receive the order once, directly against the fake's seed state,
	// so the second call below hits domain.ReceivePurchaseOrder's
	// already-received guard.
	preReceived := *f.order
	if err := preReceived.MarkAsReceived(now); err != nil {
		t.Fatalf("MarkAsReceived (fixture setup): %v", err)
	}
	f.uow.state.purchaseOrders[f.order.ID()] = &preReceived

	// Act
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now.Add(time.Hour))

	// Assert
	if err == nil {
		t.Fatal("ReceivePurchaseOrder: expected an error, got nil")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	snapshot := f.uow.snapshot()
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
	if len(snapshot.transactions) != 0 {
		t.Errorf("transactions = %v, want empty", snapshot.transactions)
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty", snapshot.events)
	}
	if snapshot.stockItems[f.stockItem1.ID()].Quantity().Thousandths() != 0 {
		t.Error("stock item 1 was mutated despite the order already being received")
	}
}
