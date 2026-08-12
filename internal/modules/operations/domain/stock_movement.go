package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// StockMovement is an immutable log entry recording a single change to a
// StockItem's quantity on hand -- an inbound receipt or an outbound
// withdrawal. It is append-only by design: once constructed, a
// StockMovement exposes only getters and can never be mutated. There is no
// setter and no pointer-receiver method anywhere in this file; that
// omission is the enforcement mechanism, not a runtime check (see
// TestStockMovement_IsImmutableByDesign in stock_movement_test.go for the
// documented rationale).
type StockMovement struct {
	id           ID
	tenantID     tenant.TenantID
	stockItemID  ID
	movementType StockMovementType
	quantity     Quantity
	reason       string
	createdAt    time.Time
}

// NewStockMovement builds a StockMovement, validating that reason is
// non-empty, quantity is non-zero, and movementType is one of the known
// stock movement types.
func NewStockMovement(
	id ID,
	tenantID tenant.TenantID,
	stockItemID ID,
	movementType StockMovementType,
	quantity Quantity,
	reason string,
) (StockMovement, error) {
	if reason == "" {
		return StockMovement{}, errs.New(errs.Invalid, "invalid_stock_movement_reason",
			"stock movement reason must not be empty", nil)
	}
	if quantity.Thousandths() == 0 {
		return StockMovement{}, errs.New(errs.Invalid, "invalid_stock_movement_quantity",
			"stock movement quantity must not be zero", nil)
	}
	if !movementType.Valid() {
		return StockMovement{}, errs.New(errs.Invalid, "invalid_stock_movement_type",
			"stock movement type is not one of the known values", nil)
	}

	return StockMovement{
		id:           id,
		tenantID:     tenantID,
		stockItemID:  stockItemID,
		movementType: movementType,
		quantity:     quantity,
		reason:       reason,
		createdAt:    time.Now().UTC(),
	}, nil
}

// ID returns the stock movement's identity.
func (m StockMovement) ID() ID { return m.id }

// TenantID returns the tenant the stock movement belongs to.
func (m StockMovement) TenantID() tenant.TenantID { return m.tenantID }

// StockItemID returns the ID of the StockItem this movement applies to.
func (m StockMovement) StockItemID() ID { return m.stockItemID }

// MovementType returns whether this is an inbound or outbound movement.
func (m StockMovement) MovementType() StockMovementType { return m.movementType }

// Quantity returns the (always non-zero) quantity moved.
func (m StockMovement) Quantity() Quantity { return m.quantity }

// Reason returns the human-readable reason recorded for this movement.
func (m StockMovement) Reason() string { return m.reason }

// CreatedAt returns the timestamp this movement was recorded at.
func (m StockMovement) CreatedAt() time.Time { return m.createdAt }
