package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Order is a tab opened against one Table: its line items (each priced,
// per unit, at the moment the order was created -- see OrderItem's own doc
// comment for why that price is FROZEN and never recalculated, even if the
// referenced product's catalog price later changes), an optional
// customer name/phone (only ever populated by the public cardápio flow --
// see NewOrder's own doc comment), and a status walking a fixed state
// machine (see the transition methods below). It is an aggregate root
// guarded by an optimistic-concurrency Version, incremented on every
// successful status transition.
//
// This phase deliberately models NO payment/billing concept at all: every
// Order is implicitly "a pagar na mesa depois" (to be settled at the table
// later) -- see this module's final report for that explicit, confirmed
// product-scope decision. Order also never references
// internal/modules/crm.Customer: a diner's name/phone, when present, live
// directly on THIS aggregate, never cross-referenced against crm's own
// customer records (again, a confirmed product decision, not an oversight).
type Order struct {
	id            ID
	tenantID      tenant.TenantID
	tableID       ID
	customerName  *string
	customerPhone *string
	items         []OrderItem
	status        OrderStatus
	version       int64
	createdAt     time.Time
	updatedAt     time.Time
}

// initialOrderVersion is the Version a freshly constructed Order starts
// at, before any mutation.
const initialOrderVersion = 1

// NewOrder builds a brand-new Order, validating that tableID is not the
// zero ID, that items contains at least one line, that no two items
// reference the same product, and that customerName/customerPhone -- both
// optional -- are not present-but-blank (mirroring
// internal/modules/crm/domain.NewCustomer's identical guard against a
// pointer to "" being mistaken for "not provided").
//
// The new order ALWAYS starts at OrderStatusPendenteConfirmacao, at
// Version 1 -- this is not a constructor parameter, because every Order in
// this phase is born from the public, unauthenticated cardápio flow (see
// ../application's CreateOrder), and this module's confirmed product
// decision is that such an order must NOT go straight to the kitchen: an
// authenticated staff member must explicitly confirm it first (see
// ../application's ConfirmOrder) before it can advance any further. There
// is deliberately no "create an already-confirmed order" code path in this
// phase.
//
// items is defensively copied so that later mutation of the caller's
// slice cannot reach back into the constructed Order.
func NewOrder(id ID, tenantID tenant.TenantID, tableID ID, customerName, customerPhone *string, items []OrderItem) (Order, error) {
	if tableID.IsZero() {
		return Order{}, errs.New(errs.Invalid, "invalid_order_table_id",
			"order table id must not be zero", nil)
	}
	if len(items) == 0 {
		return Order{}, errs.New(errs.Invalid, "invalid_order_items",
			"order must have at least one item", nil)
	}
	if customerName != nil && strings.TrimSpace(*customerName) == "" {
		return Order{}, errs.New(errs.Invalid, "invalid_order_customer_name",
			"order customer name must not be blank when provided", nil)
	}
	if customerPhone != nil && strings.TrimSpace(*customerPhone) == "" {
		return Order{}, errs.New(errs.Invalid, "invalid_order_customer_phone",
			"order customer phone must not be blank when provided", nil)
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, dup := seen[item.productID]; dup {
			return Order{}, errs.New(errs.Invalid, "duplicate_order_item",
				"order must not repeat the same product across multiple items", nil)
		}
		seen[item.productID] = struct{}{}
	}

	itemsCopy := make([]OrderItem, len(items))
	copy(itemsCopy, items)

	now := time.Now().UTC()
	return Order{
		id:            id,
		tenantID:      tenantID,
		tableID:       tableID,
		customerName:  customerName,
		customerPhone: customerPhone,
		items:         itemsCopy,
		status:        OrderStatusPendenteConfirmacao,
		version:       initialOrderVersion,
		createdAt:     now,
		updatedAt:     now,
	}, nil
}

// ID returns the order's identity.
func (o Order) ID() ID { return o.id }

// TenantID returns the tenant the order belongs to.
func (o Order) TenantID() tenant.TenantID { return o.tenantID }

// TableID returns the ID of the Table this order was opened against.
func (o Order) TableID() ID { return o.tableID }

// CustomerName returns the diner's name, or nil if none was provided
// (always nil for an order that did not come from the public cardápio
// flow).
func (o Order) CustomerName() *string { return o.customerName }

// CustomerPhone returns the diner's phone number, or nil if none was
// provided.
func (o Order) CustomerPhone() *string { return o.customerPhone }

// Items returns a defensive copy of the order's line items -- the caller
// cannot mutate the aggregate's internal slice through it.
func (o Order) Items() []OrderItem {
	itemsCopy := make([]OrderItem, len(o.items))
	copy(itemsCopy, o.items)
	return itemsCopy
}

// Status returns the order's current lifecycle status.
func (o Order) Status() OrderStatus { return o.status }

// Version returns the order's optimistic-concurrency version, starting at
// 1 and incrementing by exactly 1 on every successful status transition.
func (o Order) Version() int64 { return o.version }

// CreatedAt returns the timestamp the order was created at.
func (o Order) CreatedAt() time.Time { return o.createdAt }

// UpdatedAt returns the timestamp of the order's last successful
// transition (or its construction time, if never mutated).
func (o Order) UpdatedAt() time.Time { return o.updatedAt }

// Total returns the order's total: the sum, across all items, of each
// item's LineTotal. It returns an error (propagated from Money.Add/MulInt)
// only in the extremely unlikely event that sum would overflow an int64
// number of cents.
func (o Order) Total() (Money, error) {
	total := Money{}
	for _, item := range o.items {
		lineTotal, err := item.LineTotal()
		if err != nil {
			return Money{}, err
		}
		var err2 error
		total, err2 = total.Add(lineTotal)
		if err2 != nil {
			return Money{}, err2
		}
	}
	return total, nil
}

// transition is the shared implementation behind every state-machine
// method below (Confirm, StartPreparing, MarkReady, MarkDelivered,
// Cancel): it succeeds -- advancing status to target, incrementing
// version, and refreshing updatedAt -- only if the aggregate's CURRENT
// status is one of allowedFrom; otherwise it returns an *errs.Error of
// Kind errs.Conflict and leaves the aggregate COMPLETELY untouched (status,
// version and updatedAt all unchanged), mirroring
// internal/modules/operations/domain.PurchaseOrder.MarkAsReceived's own
// documented guard exactly.
func (o *Order) transition(target OrderStatus, code string, allowedFrom ...OrderStatus) error {
	for _, from := range allowedFrom {
		if o.status == from {
			o.status = target
			o.version++
			o.updatedAt = time.Now().UTC()
			return nil
		}
	}
	return errs.New(errs.Conflict, code,
		fmt.Sprintf("order cannot transition from %q to %q", o.status, target), nil)
}

// Confirm transitions the order from OrderStatusPendenteConfirmacao to
// OrderStatusConfirmado -- the authenticated-staff action
// (../application.ConfirmOrder) that lets a QR-code order actually enter
// the kitchen queue. See this module's product decision on NewOrder for
// why every public order needs this explicit step.
func (o *Order) Confirm() error {
	return o.transition(OrderStatusConfirmado, "order_invalid_confirm_transition",
		OrderStatusPendenteConfirmacao)
}

// StartPreparing transitions the order from OrderStatusConfirmado to
// OrderStatusEmPreparo.
func (o *Order) StartPreparing() error {
	return o.transition(OrderStatusEmPreparo, "order_invalid_start_preparing_transition",
		OrderStatusConfirmado)
}

// MarkReady transitions the order from OrderStatusEmPreparo to
// OrderStatusPronto.
func (o *Order) MarkReady() error {
	return o.transition(OrderStatusPronto, "order_invalid_mark_ready_transition",
		OrderStatusEmPreparo)
}

// MarkDelivered transitions the order from OrderStatusPronto to
// OrderStatusEntregue -- the final, terminal transition.
func (o *Order) MarkDelivered() error {
	return o.transition(OrderStatusEntregue, "order_invalid_mark_delivered_transition",
		OrderStatusPronto)
}

// Cancel transitions the order to OrderStatusCancelado from ANY
// non-terminal status (OrderStatusPendenteConfirmacao, OrderStatusConfirmado,
// OrderStatusEmPreparo, OrderStatusPronto) -- per this module's spec, an
// order may be cancelled at any point before delivery. An order that is
// already OrderStatusEntregue or already OrderStatusCancelado cannot be
// cancelled again.
func (o *Order) Cancel() error {
	return o.transition(OrderStatusCancelado, "order_invalid_cancel_transition",
		OrderStatusPendenteConfirmacao, OrderStatusConfirmado, OrderStatusEmPreparo, OrderStatusPronto)
}
