package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// centsToCurrency converts a Money's int64-cents representation into a
// float64 whole-currency amount (e.g. 350 cents -> 3.5), matching the scale
// contracts.PurchaseOrderReceivedPayload.ReceivedTotal and
// contracts.TransactionRecordedPayload.Amount are documented to carry.
func centsToCurrency(m Money) float64 {
	return float64(m.Cents()) / 100.0
}

// thousandthsToUnits converts a Quantity's int64-thousandths representation
// into a float64 whole-unit amount (e.g. 2000 thousandths -> 2.0), matching
// the scale contracts.StockItemMovementRegisteredPayload.QuantityDelta is
// documented to carry.
func thousandthsToUnits(q Quantity) float64 {
	return float64(q.Thousandths()) / 1000.0
}

// ReceiveResult is everything a successful ReceivePurchaseOrder call
// produced and that the application layer (a future phase) must persist,
// atomically, within a single database transaction. ReceiveResult carries no
// behavior of its own -- it is a pure data bag, produced without any I/O.
type ReceiveResult struct {
	// Order is the same PurchaseOrder pointer passed into
	// ReceivePurchaseOrder, now transitioned to
	// PurchaseOrderStatusRecebido.
	Order *PurchaseOrder
	// StockItems holds only the StockItems that were mutated (received
	// into), ordered ascending by stock item ID -- see
	// ReceivePurchaseOrder's doc comment for why that ordering is required.
	StockItems []*StockItem
	// Movements holds one StockMovement per received item, in the same
	// ascending-by-stock-item-ID order as StockItems.
	Movements []StockMovement
	// Transaction is the single expense Transaction recording the order's
	// total cost.
	Transaction Transaction
	// Events holds every domain event this operation raised: one
	// EventStockItemMovementRegistered per item, one
	// EventPurchaseOrderReceived, and one EventTransactionRecorded.
	Events []outbox.DomainEvent
}

// ReceivePurchaseOrder is the domain service orchestrating a purchase
// order's receipt across all four aggregates it touches (PurchaseOrder,
// StockItem, StockMovement, Transaction) as a single logical operation. It
// performs zero I/O: order and stockItems must already be loaded into
// memory by the caller (a future application-layer use case), and the
// returned ReceiveResult is handed back for that same caller to persist,
// atomically, within one database transaction. now is the operation's
// timestamp, supplied by the caller rather than read from time.Now() here,
// so this function stays a pure, deterministic computation over its inputs.
//
// ReceivePurchaseOrder first calls order.MarkAsReceived(now). If that fails
// (the order was already received, or was cancelled), ReceivePurchaseOrder
// returns that error immediately and the zero ReceiveResult -- nothing
// partially computed ever leaks out.
//
// If MarkAsReceived succeeds, ReceivePurchaseOrder then requires stockItems
// to contain an entry for every item on the order; a missing entry is
// treated as a caller bug (the application layer should have loaded every
// referenced StockItem before calling in) and reported as an *errs.Error of
// Kind errs.Invalid. That check runs, for every item, before any StockItem
// is mutated, so a missing entry can never leave some StockItems in
// stockItems received into while others were not: on this error path no
// StockItem in the map is touched at all.
//
// Once every referenced StockItem is confirmed present, ReceivePurchaseOrder
// calls StockItem.Receive on each one (summing the ordered quantity into
// stock on hand) and builds a matching StockMovement, whose Reason is
// exactly "Recebimento do pedido de compra <order id>" -- a fixed format the
// existing frontend depends on. It then computes order.Total() and builds a
// single expense Transaction (category "compras") referencing the order,
// plus one EventStockItemMovementRegistered per item, one
// EventPurchaseOrderReceived, and one EventTransactionRecorded.
//
// The returned StockItems and Movements slices are always sorted ascending
// by stock item ID, regardless of the order the items appear in on the
// PurchaseOrder itself: a future phase locks the underlying rows in this
// same order to avoid a lock-ordering deadlock between two concurrent
// receipts sharing a subset of stock items, so this ordering is a load-
// bearing part of the contract, not a cosmetic detail.
func ReceivePurchaseOrder(order *PurchaseOrder, stockItems map[ID]*StockItem, now time.Time) (ReceiveResult, error) {
	if err := order.MarkAsReceived(now); err != nil {
		return ReceiveResult{}, err
	}

	items := order.Items()
	sort.Slice(items, func(i, j int) bool {
		return items[i].StockItemID().String() < items[j].StockItemID().String()
	})

	resolved, err := resolveOrderedStockItems(items, stockItems)
	if err != nil {
		return ReceiveResult{}, err
	}

	receivedStockItems, movements, movementEvents, err := receiveStockItems(order, resolved, items, now)
	if err != nil {
		return ReceiveResult{}, err
	}

	total, err := order.Total()
	if err != nil {
		return ReceiveResult{}, err
	}

	transaction, transactionEvent, orderEvent, err := recordReceipt(order, total, now)
	if err != nil {
		return ReceiveResult{}, err
	}

	events := make([]outbox.DomainEvent, 0, len(movementEvents)+2)
	events = append(events, movementEvents...)
	events = append(events, orderEvent, transactionEvent)

	return ReceiveResult{
		Order:       order,
		StockItems:  receivedStockItems,
		Movements:   movements,
		Transaction: transaction,
		Events:      events,
	}, nil
}

// resolveOrderedStockItems looks up, in items order (already sorted
// ascending by stock item ID), the StockItem each PurchaseOrderItem refers
// to within stockItems. It returns an *errs.Error of Kind errs.Invalid on
// the first missing entry, without ever mutating any StockItem in
// stockItems -- this lookup pass is read-only by construction, which is
// exactly what guarantees a missing entry can never leave some StockItems
// received into while others were not.
func resolveOrderedStockItems(items []PurchaseOrderItem, stockItems map[ID]*StockItem) ([]*StockItem, error) {
	resolved := make([]*StockItem, len(items))
	for i, item := range items {
		stockItem, ok := stockItems[item.StockItemID()]
		if !ok {
			return nil, errs.New(errs.Invalid, "missing_stock_item",
				"purchase order references a stock item that was not loaded",
				fmt.Errorf("no stock item found in the provided map for stock item id %s", item.StockItemID()))
		}
		resolved[i] = stockItem
	}
	return resolved, nil
}

// receiveStockItems mutates each resolved StockItem via Receive (summing in
// the ordered quantity) and builds the matching StockMovement and
// EventStockItemMovementRegistered, in the same order as resolved/items
// (already sorted ascending by stock item ID).
func receiveStockItems(
	order *PurchaseOrder,
	resolved []*StockItem,
	items []PurchaseOrderItem,
	now time.Time,
) ([]*StockItem, []StockMovement, []outbox.DomainEvent, error) {
	stockItemsOut := make([]*StockItem, 0, len(items))
	movements := make([]StockMovement, 0, len(items))
	events := make([]outbox.DomainEvent, 0, len(items))

	for i, item := range items {
		stockItem := resolved[i]

		if err := stockItem.Receive(item.Quantity()); err != nil {
			return nil, nil, nil, err
		}

		movement, err := NewStockMovement(
			NewID(),
			stockItem.TenantID(),
			stockItem.ID(),
			StockMovementTypeEntrada,
			item.Quantity(),
			fmt.Sprintf("Recebimento do pedido de compra %s", order.ID()),
		)
		if err != nil {
			return nil, nil, nil, err
		}

		event, err := outbox.NewDomainEvent(
			stockItem.TenantID(),
			contracts.EventStockItemMovementRegistered,
			stockItem.ID().String(),
			now,
			contracts.StockItemMovementRegisteredPayload{
				StockItemID:   stockItem.ID().String(),
				MovementID:    movement.ID().String(),
				QuantityDelta: thousandthsToUnits(item.Quantity()),
				OccurredAt:    now,
			},
		)
		if err != nil {
			return nil, nil, nil, err
		}

		stockItemsOut = append(stockItemsOut, stockItem)
		movements = append(movements, movement)
		events = append(events, event)
	}

	return stockItemsOut, movements, events, nil
}

// recordReceipt builds the expense Transaction recording total, plus its
// EventTransactionRecorded and the order's EventPurchaseOrderReceived.
func recordReceipt(order *PurchaseOrder, total Money, now time.Time) (Transaction, outbox.DomainEvent, outbox.DomainEvent, error) {
	orderID := order.ID()
	transaction, err := NewTransaction(
		NewID(),
		order.TenantID(),
		TransactionTypeDespesa,
		TransactionCategoryCompras,
		fmt.Sprintf("Pedido de compra %s", order.ID()),
		total,
		now,
		&orderID,
	)
	if err != nil {
		return Transaction{}, outbox.DomainEvent{}, outbox.DomainEvent{}, err
	}

	orderEvent, err := outbox.NewDomainEvent(
		order.TenantID(),
		contracts.EventPurchaseOrderReceived,
		order.ID().String(),
		now,
		contracts.PurchaseOrderReceivedPayload{
			PurchaseOrderID: order.ID().String(),
			SupplierID:      order.SupplierID().String(),
			ReceivedTotal:   centsToCurrency(total),
			OccurredAt:      now,
		},
	)
	if err != nil {
		return Transaction{}, outbox.DomainEvent{}, outbox.DomainEvent{}, err
	}

	transactionEvent, err := outbox.NewDomainEvent(
		transaction.TenantID(),
		contracts.EventTransactionRecorded,
		transaction.ID().String(),
		now,
		contracts.TransactionRecordedPayload{
			TransactionID: transaction.ID().String(),
			Amount:        -centsToCurrency(total),
			OccurredAt:    now,
		},
	)
	if err != nil {
		return Transaction{}, outbox.DomainEvent{}, outbox.DomainEvent{}, err
	}

	return transaction, transactionEvent, orderEvent, nil
}
