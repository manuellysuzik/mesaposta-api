package domain_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// assertZeroReceiveResult fails the test unless result is the zero
// ReceiveResult in every field. ReceiveResult is not comparable with == (it
// embeds slices), so this checks each field individually rather than
// comparing against a ReceiveResult{} literal.
func assertZeroReceiveResult(t *testing.T, result domain.ReceiveResult) {
	t.Helper()
	if result.Order != nil {
		t.Errorf("result.Order = %v, want nil", result.Order)
	}
	if result.StockItems != nil {
		t.Errorf("result.StockItems = %v, want nil", result.StockItems)
	}
	if result.Movements != nil {
		t.Errorf("result.Movements = %v, want nil", result.Movements)
	}
	if (result.Transaction != domain.Transaction{}) {
		t.Errorf("result.Transaction = %+v, want the zero Transaction", result.Transaction)
	}
	if result.Events != nil {
		t.Errorf("result.Events = %v, want nil", result.Events)
	}
}

// newReceivingTestStockItem builds a valid StockItem with the given starting
// quantity, ready to be Received into by ReceivePurchaseOrder.
func newReceivingTestStockItem(t *testing.T, startQty int64) *domain.StockItem {
	t.Helper()
	item, err := domain.NewStockItem(
		domain.NewID(),
		mustTenantID(t),
		"Item de teste",
		domain.StockUnitKg,
		mustQuantity(t, startQty),
		mustQuantity(t, 0),
		mustMoney(t, 100),
		nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	return &item
}

// receivingFixture bundles a PurchaseOrder with two items and the matching
// stock items map, ready for ReceivePurchaseOrder happy-path tests.
type receivingFixture struct {
	order        domain.PurchaseOrder
	stockItemA   *domain.StockItem
	stockItemB   *domain.StockItem
	stockItemAID domain.ID
	stockItemBID domain.ID
	orderedQtyA  domain.Quantity
	orderedQtyB  domain.Quantity
}

// newReceivingFixture builds a pending PurchaseOrder with two items (2.000
// units @ R$3.50/unit and 1.500 units @ R$9.00/unit, matching
// purchase_order_test.go's known-value setup) plus matching StockItems
// starting with stock on hand, keyed by their stock item IDs.
func newReceivingFixture(t *testing.T) receivingFixture {
	t.Helper()

	stockItemA := newReceivingTestStockItem(t, 10000)
	stockItemB := newReceivingTestStockItem(t, 5000)

	orderedQtyA := mustQuantity(t, 2000)
	orderedQtyB := mustQuantity(t, 1500)
	costPerUnitA := mustMoney(t, 350)
	costPerUnitB := mustMoney(t, 900)

	items := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, stockItemA.ID(), orderedQtyA, costPerUnitA),
		mustPurchaseOrderItem(t, stockItemB.ID(), orderedQtyB, costPerUnitB),
	}
	order, err := domain.NewPurchaseOrder(domain.NewID(), mustTenantID(t), domain.NewID(), items)
	if err != nil {
		t.Fatalf("NewPurchaseOrder returned unexpected error: %v", err)
	}

	return receivingFixture{
		order:        order,
		stockItemA:   stockItemA,
		stockItemB:   stockItemB,
		stockItemAID: stockItemA.ID(),
		stockItemBID: stockItemB.ID(),
		orderedQtyA:  orderedQtyA,
		orderedQtyB:  orderedQtyB,
	}
}

// stockItemsMap builds the map[domain.ID]*domain.StockItem ReceivePurchaseOrder
// expects, from the fixture's two stock items.
func (f receivingFixture) stockItemsMap() map[domain.ID]*domain.StockItem {
	return map[domain.ID]*domain.StockItem{
		f.stockItemAID: f.stockItemA,
		f.stockItemBID: f.stockItemB,
	}
}

func TestReceivePurchaseOrder_HappyPath_TwoItems_ProducesExpectedMovementsStockItemsTransactionAndEvents(t *testing.T) {
	// Arrange
	f := newReceivingFixture(t)
	now := time.Now().UTC()
	beforeQtyA := f.stockItemA.Quantity()
	beforeQtyB := f.stockItemB.Quantity()

	wantTotal, totalErr := f.order.Total()
	if totalErr != nil {
		t.Fatalf("test setup: order.Total() returned unexpected error: %v", totalErr)
	}

	// Act
	result, err := domain.ReceivePurchaseOrder(&f.order, f.stockItemsMap(), now)

	// Assert: no error, order transitioned.
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder returned unexpected error: %v", err)
	}
	if result.Order == nil {
		t.Fatal("result.Order is nil, want the received PurchaseOrder")
	}
	if result.Order.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("result.Order.Status() = %v, want %v", result.Order.Status(), domain.PurchaseOrderStatusRecebido)
	}

	// Assert: StockItems -- both mutated, quantities summed correctly.
	if len(result.StockItems) != 2 {
		t.Fatalf("len(result.StockItems) = %d, want 2", len(result.StockItems))
	}
	wantQtyA, addErrA := beforeQtyA.Add(f.orderedQtyA)
	if addErrA != nil {
		t.Fatalf("test setup: beforeQtyA.Add(orderedQtyA) returned unexpected error: %v", addErrA)
	}
	wantQtyB, addErrB := beforeQtyB.Add(f.orderedQtyB)
	if addErrB != nil {
		t.Fatalf("test setup: beforeQtyB.Add(orderedQtyB) returned unexpected error: %v", addErrB)
	}
	if f.stockItemA.Quantity() != wantQtyA {
		t.Errorf("stockItemA.Quantity() = %v, want %v", f.stockItemA.Quantity(), wantQtyA)
	}
	if f.stockItemB.Quantity() != wantQtyB {
		t.Errorf("stockItemB.Quantity() = %v, want %v", f.stockItemB.Quantity(), wantQtyB)
	}

	// Assert: Movements -- one per item, type entrada, exact reason text.
	if len(result.Movements) != 2 {
		t.Fatalf("len(result.Movements) = %d, want 2", len(result.Movements))
	}
	wantReason := fmt.Sprintf("Recebimento do pedido de compra %s", f.order.ID())
	for _, movement := range result.Movements {
		if movement.MovementType() != domain.StockMovementTypeEntrada {
			t.Errorf("movement.MovementType() = %v, want %v", movement.MovementType(), domain.StockMovementTypeEntrada)
		}
		if movement.Reason() != wantReason {
			t.Errorf("movement.Reason() = %q, want %q", movement.Reason(), wantReason)
		}
		switch movement.StockItemID() {
		case f.stockItemAID:
			if movement.Quantity() != f.orderedQtyA {
				t.Errorf("movement (A).Quantity() = %v, want %v", movement.Quantity(), f.orderedQtyA)
			}
		case f.stockItemBID:
			if movement.Quantity() != f.orderedQtyB {
				t.Errorf("movement (B).Quantity() = %v, want %v", movement.Quantity(), f.orderedQtyB)
			}
		default:
			t.Errorf("movement.StockItemID() = %v, want one of %v or %v", movement.StockItemID(), f.stockItemAID, f.stockItemBID)
		}
	}

	// Assert: exactly 1 Transaction, amount == order.Total(), referenceID == order.ID().
	if result.Transaction.Amount() != wantTotal {
		t.Errorf("result.Transaction.Amount() = %v, want %v", result.Transaction.Amount(), wantTotal)
	}
	if result.Transaction.ReferenceID() == nil || *result.Transaction.ReferenceID() != f.order.ID() {
		t.Errorf("result.Transaction.ReferenceID() = %v, want %v", result.Transaction.ReferenceID(), f.order.ID())
	}
	if result.Transaction.TransactionType() != domain.TransactionTypeDespesa {
		t.Errorf("result.Transaction.TransactionType() = %v, want %v", result.Transaction.TransactionType(), domain.TransactionTypeDespesa)
	}
	if result.Transaction.Category() != domain.TransactionCategoryCompras {
		t.Errorf("result.Transaction.Category() = %v, want %v", result.Transaction.Category(), domain.TransactionCategoryCompras)
	}
	wantDescription := fmt.Sprintf("Pedido de compra %s", f.order.ID())
	if result.Transaction.Description() != wantDescription {
		t.Errorf("result.Transaction.Description() = %q, want %q", result.Transaction.Description(), wantDescription)
	}

	// Assert: events -- 2 movement events + 1 purchase order event + 1
	// transaction event = 4 total, each with the expected name and payload.
	if len(result.Events) != 4 {
		t.Fatalf("len(result.Events) = %d, want 4", len(result.Events))
	}

	var movementEvents, orderEvents, transactionEvents int
	for _, event := range result.Events {
		switch event.Name {
		case contracts.EventStockItemMovementRegistered:
			movementEvents++
			payload, ok := event.Payload.(contracts.StockItemMovementRegisteredPayload)
			if !ok {
				t.Fatalf("event payload type = %T, want contracts.StockItemMovementRegisteredPayload", event.Payload)
			}
			if payload.StockItemID != f.stockItemAID.String() && payload.StockItemID != f.stockItemBID.String() {
				t.Errorf("payload.StockItemID = %q, want one of %q or %q", payload.StockItemID, f.stockItemAID, f.stockItemBID)
			}
			if payload.QuantityDelta <= 0 {
				t.Errorf("payload.QuantityDelta = %v, want a positive value (inbound movement)", payload.QuantityDelta)
			}
			if !payload.OccurredAt.Equal(now) {
				t.Errorf("payload.OccurredAt = %v, want %v", payload.OccurredAt, now)
			}
		case contracts.EventPurchaseOrderReceived:
			orderEvents++
			payload, ok := event.Payload.(contracts.PurchaseOrderReceivedPayload)
			if !ok {
				t.Fatalf("event payload type = %T, want contracts.PurchaseOrderReceivedPayload", event.Payload)
			}
			if payload.PurchaseOrderID != f.order.ID().String() {
				t.Errorf("payload.PurchaseOrderID = %q, want %q", payload.PurchaseOrderID, f.order.ID())
			}
			if payload.SupplierID != f.order.SupplierID().String() {
				t.Errorf("payload.SupplierID = %q, want %q", payload.SupplierID, f.order.SupplierID())
			}
			wantReceivedTotal := float64(wantTotal.Cents()) / 100.0
			if payload.ReceivedTotal != wantReceivedTotal {
				t.Errorf("payload.ReceivedTotal = %v, want %v", payload.ReceivedTotal, wantReceivedTotal)
			}
			if !payload.OccurredAt.Equal(now) {
				t.Errorf("payload.OccurredAt = %v, want %v", payload.OccurredAt, now)
			}
		case contracts.EventTransactionRecorded:
			transactionEvents++
			payload, ok := event.Payload.(contracts.TransactionRecordedPayload)
			if !ok {
				t.Fatalf("event payload type = %T, want contracts.TransactionRecordedPayload", event.Payload)
			}
			if payload.TransactionID != result.Transaction.ID().String() {
				t.Errorf("payload.TransactionID = %q, want %q", payload.TransactionID, result.Transaction.ID())
			}
			if payload.Amount >= 0 {
				t.Errorf("payload.Amount = %v, want a negative value (expense)", payload.Amount)
			}
			if !payload.OccurredAt.Equal(now) {
				t.Errorf("payload.OccurredAt = %v, want %v", payload.OccurredAt, now)
			}
		default:
			t.Errorf("unexpected event name %q", event.Name)
		}
	}
	if movementEvents != 2 {
		t.Errorf("movement events = %d, want 2", movementEvents)
	}
	if orderEvents != 1 {
		t.Errorf("order events = %d, want 1", orderEvents)
	}
	if transactionEvents != 1 {
		t.Errorf("transaction events = %d, want 1", transactionEvents)
	}
}

func TestReceivePurchaseOrder_KnownTotal_MatchesManualCalculation(t *testing.T) {
	// Arrange: item A = 2.000 units @ R$3.50/unit = 700 cents;
	// item B = 1.500 units @ R$9.00/unit = 1350 cents. Total = 2050 cents.
	f := newReceivingFixture(t)

	// Act
	result, err := domain.ReceivePurchaseOrder(&f.order, f.stockItemsMap(), time.Now().UTC())

	// Assert
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder returned unexpected error: %v", err)
	}
	if result.Transaction.Amount().Cents() != 2050 {
		t.Errorf("result.Transaction.Amount().Cents() = %d, want 2050", result.Transaction.Amount().Cents())
	}
}

func TestReceivePurchaseOrder_OrderAlreadyReceived_ReturnsConflictAndZeroResult(t *testing.T) {
	// Arrange
	f := newReceivingFixture(t)
	firstNow := time.Now().UTC()
	if _, err := domain.ReceivePurchaseOrder(&f.order, f.stockItemsMap(), firstNow); err != nil {
		t.Fatalf("first ReceivePurchaseOrder returned unexpected error: %v", err)
	}

	// Act: try to receive the same, already-received order again.
	result, err := domain.ReceivePurchaseOrder(&f.order, f.stockItemsMap(), firstNow.Add(1*time.Hour))

	// Assert
	if err == nil {
		t.Fatal("second ReceivePurchaseOrder returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
	assertZeroReceiveResult(t, result)
}

func TestReceivePurchaseOrder_MissingStockItemInMap_ReturnsInvalidAndDoesNotMutateAnyStockItem(t *testing.T) {
	// Arrange: stockItemsMap only contains stockItemA -- stockItemB (the
	// second item in the order, so at least one item would have been
	// processed first if item order were preserved) is missing.
	f := newReceivingFixture(t)
	beforeQtyA := f.stockItemA.Quantity()
	beforeVersionA := f.stockItemA.Version()
	incompleteMap := map[domain.ID]*domain.StockItem{
		f.stockItemAID: f.stockItemA,
	}

	// Act
	result, err := domain.ReceivePurchaseOrder(&f.order, incompleteMap, time.Now().UTC())

	// Assert: the error.
	if err == nil {
		t.Fatal("ReceivePurchaseOrder with a missing stock item returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	assertZeroReceiveResult(t, result)

	// Assert: stockItemA (present in the map) was NOT mutated, even though
	// it could have been processed before the missing item was discovered.
	if f.stockItemA.Quantity() != beforeQtyA {
		t.Errorf("stockItemA.Quantity() = %v after failed ReceivePurchaseOrder, want unchanged %v", f.stockItemA.Quantity(), beforeQtyA)
	}
	if f.stockItemA.Version() != beforeVersionA {
		t.Errorf("stockItemA.Version() = %d after failed ReceivePurchaseOrder, want unchanged %d", f.stockItemA.Version(), beforeVersionA)
	}
}

func TestReceivePurchaseOrder_ResultOrder_IsDeterministicRegardlessOfItemOrderInPurchaseOrder(t *testing.T) {
	// Arrange: two purchase orders referencing the SAME three stock item
	// IDs but listing their items in opposite order, plus a fresh, distinct
	// set of StockItems for each run (so the two ReceivePurchaseOrder calls
	// don't interfere with each other's state). ReceivePurchaseOrder must
	// sort its own output by stock item ID rather than preserving whatever
	// order the items happen to appear in on the PurchaseOrder.
	tenantID := mustTenantID(t)
	supplierID := domain.NewID()

	idA := domain.NewID()
	idB := domain.NewID()
	idC := domain.NewID()
	qty := mustQuantity(t, 1000)
	cost := mustMoney(t, 500)

	itemsAscending := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, idA, qty, cost),
		mustPurchaseOrderItem(t, idB, qty, cost),
		mustPurchaseOrderItem(t, idC, qty, cost),
	}
	itemsShuffled := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, idC, qty, cost),
		mustPurchaseOrderItem(t, idA, qty, cost),
		mustPurchaseOrderItem(t, idB, qty, cost),
	}

	orderAscending, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, supplierID, itemsAscending)
	if err != nil {
		t.Fatalf("NewPurchaseOrder (ascending) returned unexpected error: %v", err)
	}
	orderShuffled, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, supplierID, itemsShuffled)
	if err != nil {
		t.Fatalf("NewPurchaseOrder (shuffled) returned unexpected error: %v", err)
	}

	// newStockItemsFor builds a fresh map[ID]*StockItem using the given IDs,
	// so each ReceivePurchaseOrder call starts from independent state.
	newStockItemsFor := func() map[domain.ID]*domain.StockItem {
		return map[domain.ID]*domain.StockItem{
			idA: mustStockItemWithID(t, idA),
			idB: mustStockItemWithID(t, idB),
			idC: mustStockItemWithID(t, idC),
		}
	}

	// Act
	resultAscending, err := domain.ReceivePurchaseOrder(&orderAscending, newStockItemsFor(), time.Now().UTC())
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder (ascending) returned unexpected error: %v", err)
	}
	resultShuffled, err := domain.ReceivePurchaseOrder(&orderShuffled, newStockItemsFor(), time.Now().UTC())
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder (shuffled) returned unexpected error: %v", err)
	}

	// Assert: both results list StockItems and Movements in the SAME order
	// (ascending by stock item ID), regardless of the input item order.
	if len(resultAscending.StockItems) != 3 || len(resultShuffled.StockItems) != 3 {
		t.Fatalf("len(StockItems) = %d / %d, want 3 / 3", len(resultAscending.StockItems), len(resultShuffled.StockItems))
	}
	for i := range resultAscending.StockItems {
		if resultAscending.StockItems[i].ID() != resultShuffled.StockItems[i].ID() {
			t.Errorf("StockItems[%d].ID() = %v (ascending) vs %v (shuffled), want equal",
				i, resultAscending.StockItems[i].ID(), resultShuffled.StockItems[i].ID())
		}
	}
	for i := range resultAscending.Movements {
		if resultAscending.Movements[i].StockItemID() != resultShuffled.Movements[i].StockItemID() {
			t.Errorf("Movements[%d].StockItemID() = %v (ascending) vs %v (shuffled), want equal",
				i, resultAscending.Movements[i].StockItemID(), resultShuffled.Movements[i].StockItemID())
		}
	}

	// Assert: the order is strictly ascending by ID string.
	for i := 1; i < len(resultAscending.StockItems); i++ {
		if resultAscending.StockItems[i-1].ID().String() >= resultAscending.StockItems[i].ID().String() {
			t.Errorf("StockItems not sorted ascending by ID: [%d]=%v, [%d]=%v",
				i-1, resultAscending.StockItems[i-1].ID(), i, resultAscending.StockItems[i].ID())
		}
	}
}

// mustStockItemWithID builds a valid StockItem with the given id and 10.000
// units of starting stock, failing the test immediately on error.
func mustStockItemWithID(t *testing.T, id domain.ID) *domain.StockItem {
	t.Helper()
	item, err := domain.NewStockItem(
		id,
		mustTenantID(t),
		"Item de teste",
		domain.StockUnitKg,
		mustQuantity(t, 10000),
		mustQuantity(t, 0),
		mustMoney(t, 100),
		nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	return &item
}
