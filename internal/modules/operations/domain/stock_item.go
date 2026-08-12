package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// StockItem is an ingredient/supply tracked in a tenant's stock: how much
// is currently on hand, the reorder threshold, and its unit cost. It is an
// aggregate root guarded by an optimistic-concurrency Version, incremented
// on every successful mutation (Withdraw, Receive).
type StockItem struct {
	id          ID
	tenantID    tenant.TenantID
	name        string
	unit        StockUnit
	quantity    Quantity
	minQuantity Quantity
	costPerUnit Money
	supplierID  *ID
	version     int64
	updatedAt   time.Time
}

// initialStockItemVersion is the Version a freshly constructed StockItem
// starts at, before any mutation.
const initialStockItemVersion = 1

// NewStockItem builds a StockItem, validating that name is non-empty and
// unit is one of the known stock units. quantity, minQuantity and
// costPerUnit are already guaranteed non-negative by their own types
// (Quantity, Money), so no further check is needed here. supplierID is
// optional (nil means "no supplier assigned yet" -- there is no foreign key
// enforcement at this phase).
func NewStockItem(
	id ID,
	tenantID tenant.TenantID,
	name string,
	unit StockUnit,
	quantity Quantity,
	minQuantity Quantity,
	costPerUnit Money,
	supplierID *ID,
) (StockItem, error) {
	if name == "" {
		return StockItem{}, errs.New(errs.Invalid, "invalid_stock_item_name",
			"stock item name must not be empty", nil)
	}
	if !unit.Valid() {
		return StockItem{}, errs.New(errs.Invalid, "invalid_stock_unit",
			"stock item unit is not one of the known values", nil)
	}

	return StockItem{
		id:          id,
		tenantID:    tenantID,
		name:        name,
		unit:        unit,
		quantity:    quantity,
		minQuantity: minQuantity,
		costPerUnit: costPerUnit,
		supplierID:  supplierID,
		version:     initialStockItemVersion,
		updatedAt:   time.Now().UTC(),
	}, nil
}

// ID returns the stock item's identity.
func (s StockItem) ID() ID { return s.id }

// TenantID returns the tenant the stock item belongs to.
func (s StockItem) TenantID() tenant.TenantID { return s.tenantID }

// Name returns the stock item's display name.
func (s StockItem) Name() string { return s.name }

// Unit returns the stock item's unit of measure.
func (s StockItem) Unit() StockUnit { return s.unit }

// Quantity returns the stock item's current quantity on hand.
func (s StockItem) Quantity() Quantity { return s.quantity }

// MinQuantity returns the stock item's reorder threshold.
func (s StockItem) MinQuantity() Quantity { return s.minQuantity }

// CostPerUnit returns the stock item's cost per unit.
func (s StockItem) CostPerUnit() Money { return s.costPerUnit }

// SupplierID returns the stock item's assigned supplier ID, or nil if none
// is assigned.
func (s StockItem) SupplierID() *ID { return s.supplierID }

// Version returns the stock item's optimistic-concurrency version, starting
// at 1 and incrementing by exactly 1 on every successful mutation.
func (s StockItem) Version() int64 { return s.version }

// UpdatedAt returns the timestamp of the stock item's last successful
// mutation (or its construction time, if never mutated).
func (s StockItem) UpdatedAt() time.Time { return s.updatedAt }

// Withdraw removes qty from the stock item's quantity on hand (e.g. an
// ingredient consumed by a sale). qty must be non-zero.
//
// If qty exceeds the current quantity on hand, Withdraw refuses the
// operation and returns an *errs.Error of Kind errs.Conflict -- the
// aggregate is left completely untouched (quantity, version and updatedAt
// all unchanged) so a rejected withdrawal can never partially apply.
func (s *StockItem) Withdraw(qty Quantity) error {
	if qty.Thousandths() == 0 {
		return errs.New(errs.Invalid, "invalid_withdraw_quantity",
			"withdraw quantity must not be zero", nil)
	}

	remaining, err := s.quantity.Sub(qty)
	if err != nil {
		return errs.New(errs.Conflict, "insufficient_stock",
			"withdraw quantity exceeds stock item quantity on hand", err)
	}

	s.quantity = remaining
	s.version++
	s.updatedAt = time.Now().UTC()
	return nil
}

// Receive adds qty to the stock item's quantity on hand (e.g. a purchase
// order delivery). qty must be non-zero.
//
// Receive returns an *errs.Error (propagated from Quantity.Add) if the
// resulting quantity would overflow -- the aggregate is left untouched in
// that case.
func (s *StockItem) Receive(qty Quantity) error {
	if qty.Thousandths() == 0 {
		return errs.New(errs.Invalid, "invalid_receive_quantity",
			"receive quantity must not be zero", nil)
	}

	sum, err := s.quantity.Add(qty)
	if err != nil {
		return err
	}

	s.quantity = sum
	s.version++
	s.updatedAt = time.Now().UTC()
	return nil
}
