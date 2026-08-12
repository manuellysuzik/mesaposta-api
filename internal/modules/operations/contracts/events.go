package contracts

import "time"

// Event name constants for the operations module. Every name is
// dot-namespaced as "<aggregate_type>.<event>", with exactly one "." --
// internal/platform/outbox/postgres/postgres_publisher.go's aggregateTypeOf
// derives the domain_events.aggregate_type column from the prefix up to
// the first ".", so the prefix below must always match the aggregate the
// event describes (see events_test.go for the regression test pinning
// this down).
const (
	// EventStockItemMovementRegistered is published whenever a stock item's
	// quantity changes (received, sold, adjusted, ...). Payload:
	// StockItemMovementRegisteredPayload.
	EventStockItemMovementRegistered = "stock_item.movement_registered"

	// EventPurchaseOrderReceived is published when a purchase order is
	// marked received, closing the loop with the stock movements it
	// generated. Payload: PurchaseOrderReceivedPayload.
	EventPurchaseOrderReceived = "purchase_order.received"

	// EventTransactionRecorded is published whenever a financial
	// transaction (e.g. a purchase order's payment) is recorded. Payload:
	// TransactionRecordedPayload.
	EventTransactionRecorded = "transaction.recorded"
)

// StockItemMovementRegisteredPayload is the payload for
// EventStockItemMovementRegistered. QuantityDelta is signed: positive for
// inbound movements (received, adjusted up), negative for outbound ones
// (sold, adjusted down).
type StockItemMovementRegisteredPayload struct {
	StockItemID   string    `json:"stock_item_id"`
	MovementID    string    `json:"movement_id"`
	QuantityDelta float64   `json:"quantity_delta"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// PurchaseOrderReceivedPayload is the payload for
// EventPurchaseOrderReceived. ReceivedTotal is the order's total monetary
// value at the moment it was marked received.
type PurchaseOrderReceivedPayload struct {
	PurchaseOrderID string    `json:"purchase_order_id"`
	SupplierID      string    `json:"supplier_id"`
	ReceivedTotal   float64   `json:"received_total"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// TransactionRecordedPayload is the payload for EventTransactionRecorded.
// Amount is signed: positive for inbound transactions (revenue), negative
// for outbound ones (expenses, e.g. paying a purchase order).
type TransactionRecordedPayload struct {
	TransactionID string    `json:"transaction_id"`
	Amount        float64   `json:"amount"`
	OccurredAt    time.Time `json:"occurred_at"`
}
