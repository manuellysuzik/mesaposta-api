package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// PurchaseOrderItem is a single line of a PurchaseOrder: how much of a
// given stock item was ordered, and at what cost per unit. It is a value
// object with no identity of its own -- it is only ever meaningful as part
// of the PurchaseOrder that contains it.
type PurchaseOrderItem struct {
	stockItemID ID
	quantity    Quantity
	costPerUnit Money
}

// NewPurchaseOrderItem builds a PurchaseOrderItem, validating that
// stockItemID is not the zero ID and quantity is non-zero (ordering zero of
// an item makes no sense). costPerUnit is already guaranteed non-negative
// by Money's own constructor.
func NewPurchaseOrderItem(stockItemID ID, quantity Quantity, costPerUnit Money) (PurchaseOrderItem, error) {
	if stockItemID.IsZero() {
		return PurchaseOrderItem{}, errs.New(errs.Invalid, "invalid_purchase_order_item_stock_item_id",
			"purchase order item stock item ID must not be zero", nil)
	}
	if quantity.Thousandths() == 0 {
		return PurchaseOrderItem{}, errs.New(errs.Invalid, "invalid_purchase_order_item_quantity",
			"purchase order item quantity must not be zero", nil)
	}

	return PurchaseOrderItem{
		stockItemID: stockItemID,
		quantity:    quantity,
		costPerUnit: costPerUnit,
	}, nil
}

// StockItemID returns the ID of the stock item this line refers to.
func (i PurchaseOrderItem) StockItemID() ID { return i.stockItemID }

// Quantity returns the (always non-zero) quantity ordered.
func (i PurchaseOrderItem) Quantity() Quantity { return i.quantity }

// CostPerUnit returns the agreed cost per unit for this line.
func (i PurchaseOrderItem) CostPerUnit() Money { return i.costPerUnit }

// PurchaseOrder is an order placed with a supplier for one or more stock
// items. It is an aggregate root guarded by an optimistic-concurrency
// Version, incremented on every successful mutation (MarkAsReceived,
// Cancel).
type PurchaseOrder struct {
	id         ID
	tenantID   tenant.TenantID
	supplierID ID
	status     PurchaseOrderStatus
	items      []PurchaseOrderItem
	version    int64
	createdAt  time.Time
	receivedAt *time.Time
}

// initialPurchaseOrderVersion is the Version a freshly constructed
// PurchaseOrder starts at, before any mutation.
const initialPurchaseOrderVersion = 1

// NewPurchaseOrder builds a PurchaseOrder, validating that items contains
// at least one line and that no two items reference the same stock item
// (each stock item may appear at most once per order; ordering more of it
// means increasing that single line's quantity, not adding a second line).
// The new order always starts out PurchaseOrderStatusPendente at Version 1.
//
// items is defensively copied so that later mutation of the caller's slice
// cannot reach back into the constructed PurchaseOrder.
func NewPurchaseOrder(id ID, tenantID tenant.TenantID, supplierID ID, items []PurchaseOrderItem) (PurchaseOrder, error) {
	if len(items) == 0 {
		return PurchaseOrder{}, errs.New(errs.Invalid, "invalid_purchase_order_items",
			"purchase order must have at least one item", nil)
	}

	seen := make(map[ID]struct{}, len(items))
	for _, item := range items {
		if _, dup := seen[item.stockItemID]; dup {
			return PurchaseOrder{}, errs.New(errs.Invalid, "duplicate_purchase_order_item",
				"purchase order must not repeat the same stock item across multiple items", nil)
		}
		seen[item.stockItemID] = struct{}{}
	}

	itemsCopy := make([]PurchaseOrderItem, len(items))
	copy(itemsCopy, items)

	return PurchaseOrder{
		id:         id,
		tenantID:   tenantID,
		supplierID: supplierID,
		status:     PurchaseOrderStatusPendente,
		items:      itemsCopy,
		version:    initialPurchaseOrderVersion,
		createdAt:  time.Now().UTC(),
	}, nil
}

// ID returns the purchase order's identity.
func (o PurchaseOrder) ID() ID { return o.id }

// TenantID returns the tenant the purchase order belongs to.
func (o PurchaseOrder) TenantID() tenant.TenantID { return o.tenantID }

// SupplierID returns the ID of the supplier this order was placed with.
func (o PurchaseOrder) SupplierID() ID { return o.supplierID }

// Status returns the purchase order's current lifecycle status.
func (o PurchaseOrder) Status() PurchaseOrderStatus { return o.status }

// Items returns a defensive copy of the purchase order's line items -- the
// caller cannot mutate the aggregate's internal slice through it.
func (o PurchaseOrder) Items() []PurchaseOrderItem {
	itemsCopy := make([]PurchaseOrderItem, len(o.items))
	copy(itemsCopy, o.items)
	return itemsCopy
}

// Version returns the purchase order's optimistic-concurrency version,
// starting at 1 and incrementing by exactly 1 on every successful
// mutation.
func (o PurchaseOrder) Version() int64 { return o.version }

// CreatedAt returns the timestamp the purchase order was created at.
func (o PurchaseOrder) CreatedAt() time.Time { return o.createdAt }

// ReceivedAt returns the timestamp the purchase order was marked as
// received at, or nil if it has not been received yet.
func (o PurchaseOrder) ReceivedAt() *time.Time { return o.receivedAt }

// Total returns the purchase order's total cost: the sum, across all
// items, of each item's costPerUnit times its quantity. It returns an
// error (propagated from Money.Add) if that sum would overflow an int64
// number of cents -- an extremely unlikely scenario at real-world order
// sizes, but one Money.Add already guards against for every other sum in
// this codebase, so Total does not special-case its way around that
// guarantee.
func (o PurchaseOrder) Total() (Money, error) {
	total := Money{}
	for _, item := range o.items {
		lineTotal := item.costPerUnit.Mul(item.quantity)
		var err error
		total, err = total.Add(lineTotal)
		if err != nil {
			return Money{}, err
		}
	}
	return total, nil
}

// MarkAsReceived transitions the purchase order from
// PurchaseOrderStatusPendente to PurchaseOrderStatusRecebido, recording now
// as ReceivedAt and incrementing Version by 1.
//
// If the order was already received, MarkAsReceived returns an
// *errs.Error of Kind errs.Conflict (code "purchase_order_already_received")
// and leaves the aggregate completely untouched -- status, version and
// receivedAt all unchanged -- so a second, redundant call can never
// silently overwrite the original receipt timestamp. If the order was
// cancelled, MarkAsReceived similarly refuses with Kind errs.Conflict (code
// "purchase_order_cancelled") and leaves the aggregate untouched.
func (o *PurchaseOrder) MarkAsReceived(now time.Time) error {
	switch o.status {
	case PurchaseOrderStatusRecebido:
		return errs.New(errs.Conflict, "purchase_order_already_received",
			"purchase order has already been received", nil)
	case PurchaseOrderStatusCancelado:
		return errs.New(errs.Conflict, "purchase_order_cancelled",
			"purchase order was cancelled and cannot be received", nil)
	}

	o.status = PurchaseOrderStatusRecebido
	o.receivedAt = &now
	o.version++
	return nil
}

// Cancel transitions the purchase order from PurchaseOrderStatusPendente to
// PurchaseOrderStatusCancelado, incrementing Version by 1.
//
// Cancel exists both as legitimate lifecycle behavior (a pending order can
// be called off before receipt) and as the minimal way to reach the
// cancelled state needed to exercise MarkAsReceived's cancelled-order guard
// in tests, per this module's test plan.
//
// If the order was already received, or already cancelled, Cancel returns
// an *errs.Error of Kind errs.Conflict and leaves the aggregate untouched,
// mirroring MarkAsReceived's own idempotency guard.
func (o *PurchaseOrder) Cancel() error {
	switch o.status {
	case PurchaseOrderStatusRecebido:
		return errs.New(errs.Conflict, "purchase_order_already_received",
			"purchase order has already been received and cannot be cancelled", nil)
	case PurchaseOrderStatusCancelado:
		return errs.New(errs.Conflict, "purchase_order_already_cancelled",
			"purchase order has already been cancelled", nil)
	}

	o.status = PurchaseOrderStatusCancelado
	o.version++
	return nil
}
