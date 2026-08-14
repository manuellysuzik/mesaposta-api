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

// testActorID is the audit "who" every test in this file passes to
// ReceivePurchaseOrder, standing in for the JWT "sub" claim the HTTP layer
// supplies in production (see
// internal/modules/operations/infrastructure/http.actorIDFrom).
const testActorID = "user-42"

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
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID)

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
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID)

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
	// The audit trail rolls back with everything else: an entry claiming a
	// receipt that never committed would be a record of something that did
	// not happen.
	if len(snapshot.auditEntries) != 0 {
		t.Errorf("auditEntries = %v, want empty", snapshot.auditEntries)
	}
}

// TestReceivePurchaseOrder_RecordsExactlyOneAuditEntry is the audit trail's
// own acceptance test: a successful receipt writes exactly one entry, with
// the actor the caller supplied, the stable action/entity identifiers, and
// before/after snapshots that actually differ -- a "before" accidentally
// read after the domain mutation would show the post-receipt state twice,
// which is the specific bug this test exists to catch.
func TestReceivePurchaseOrder_RecordsExactlyOneAuditEntry(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()
	versionBefore := f.order.Version()

	// Act
	if err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID); err != nil {
		t.Fatalf("ReceivePurchaseOrder: unexpected error: %v", err)
	}

	// Assert
	snapshot := f.uow.snapshot()
	if len(snapshot.auditEntries) != 1 {
		t.Fatalf("len(auditEntries) = %d, want exactly 1: %+v", len(snapshot.auditEntries), snapshot.auditEntries)
	}
	entry := snapshot.auditEntries[0]

	if entry.TenantID != f.order.TenantID() {
		t.Errorf("entry.TenantID = %v, want %v", entry.TenantID, f.order.TenantID())
	}
	if entry.ActorID != testActorID {
		t.Errorf("entry.ActorID = %q, want %q", entry.ActorID, testActorID)
	}
	if entry.Action != "purchase_order.received" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "purchase_order.received")
	}
	if entry.EntityType != "purchase_order" {
		t.Errorf("entry.EntityType = %q, want %q", entry.EntityType, "purchase_order")
	}
	if entry.EntityID != f.order.ID().String() {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, f.order.ID().String())
	}
	if !entry.OccurredAt.Equal(now) {
		t.Errorf("entry.OccurredAt = %v, want %v", entry.OccurredAt, now)
	}

	before, ok := entry.Before.(map[string]any)
	if !ok {
		t.Fatalf("entry.Before = %T, want map[string]any", entry.Before)
	}
	if before["status"] != string(domain.PurchaseOrderStatusPendente) {
		t.Errorf("entry.Before[status] = %v, want %q", before["status"], domain.PurchaseOrderStatusPendente)
	}
	if before["version"] != versionBefore {
		t.Errorf("entry.Before[version] = %v, want %d", before["version"], versionBefore)
	}

	after, ok := entry.After.(map[string]any)
	if !ok {
		t.Fatalf("entry.After = %T, want map[string]any", entry.After)
	}
	if after["status"] != string(domain.PurchaseOrderStatusRecebido) {
		t.Errorf("entry.After[status] = %v, want %q", after["status"], domain.PurchaseOrderStatusRecebido)
	}
	if after["version"] != versionBefore+1 {
		t.Errorf("entry.After[version] = %v, want %d", after["version"], versionBefore+1)
	}
}

// TestReceivePurchaseOrder_EmptyActor_IsRecordedAsIs pins down the
// best-effort actor decision: an unidentifiable actor must never block the
// business operation, and must never be papered over with a placeholder --
// the empty string flows straight through, and the Postgres adapter turns
// it into SQL NULL.
func TestReceivePurchaseOrder_EmptyActor_IsRecordedAsIs(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()

	// Act
	if err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, ""); err != nil {
		t.Fatalf("ReceivePurchaseOrder with no actor: unexpected error: %v", err)
	}

	// Assert
	snapshot := f.uow.snapshot()
	if len(snapshot.auditEntries) != 1 {
		t.Fatalf("len(auditEntries) = %d, want exactly 1", len(snapshot.auditEntries))
	}
	if got := snapshot.auditEntries[0].ActorID; got != "" {
		t.Errorf("entry.ActorID = %q, want %q (no placeholder substitution)", got, "")
	}
}

// TestReceivePurchaseOrder_AuditFailure_RollsBackTheWholeUnitOfWork is the
// other half of the atomicity guarantee: the trail cannot be silently
// skipped either. If the audit write fails, the receipt it was supposed to
// describe must not commit, leaving no unrecorded mutation behind.
func TestReceivePurchaseOrder_AuditFailure_RollsBackTheWholeUnitOfWork(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()
	f.uow.errors.auditRecord = errs.New(errs.Internal, "audit_record_failed",
		"simulated audit write failure", errors.New("connection reset"))

	// Act
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID)

	// Assert
	if err == nil {
		t.Fatal("ReceivePurchaseOrder: expected an error when the audit write fails, got nil")
	}
	if !errs.IsKind(err, errs.Internal) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Internal)
	}

	snapshot := f.uow.snapshot()
	if snapshot.purchaseOrders[f.order.ID()].Status() != domain.PurchaseOrderStatusPendente {
		t.Error("purchase order was persisted despite the audit write failing")
	}
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
	if len(snapshot.transactions) != 0 {
		t.Errorf("transactions = %v, want empty", snapshot.transactions)
	}
	if len(snapshot.auditEntries) != 0 {
		t.Errorf("auditEntries = %v, want empty", snapshot.auditEntries)
	}
}

func TestReceivePurchaseOrder_PersistsPurchaseOrderBeforeAnyOtherWrite(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)
	now := time.Now().UTC()

	// Act
	if err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID); err != nil {
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
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now, testActorID)

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
	err := application.ReceivePurchaseOrder(context.Background(), f.uow, f.order.ID(), now.Add(time.Hour), testActorID)

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
