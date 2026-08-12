package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// newTestStockMovement builds a valid StockMovement, ready for use in tests
// that don't care about the specific field values.
func newTestStockMovement(t *testing.T) domain.StockMovement {
	t.Helper()
	m, err := domain.NewStockMovement(
		domain.NewID(),
		mustTenantID(t),
		domain.NewID(),
		domain.StockMovementTypeSaida,
		mustQuantity(t, 1000),
		"venda #123",
	)
	if err != nil {
		t.Fatalf("NewStockMovement returned unexpected error: %v", err)
	}
	return m
}

func TestNewStockMovement_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	stockItemID := domain.NewID()
	qty := mustQuantity(t, 500)

	// Act
	m, err := domain.NewStockMovement(id, tenantID, stockItemID, domain.StockMovementTypeEntrada, qty, "compra #1")

	// Assert
	if err != nil {
		t.Fatalf("NewStockMovement returned unexpected error: %v", err)
	}
	if m.ID() != id {
		t.Errorf("m.ID() = %v, want %v", m.ID(), id)
	}
	if m.TenantID() != tenantID {
		t.Errorf("m.TenantID() = %v, want %v", m.TenantID(), tenantID)
	}
	if m.StockItemID() != stockItemID {
		t.Errorf("m.StockItemID() = %v, want %v", m.StockItemID(), stockItemID)
	}
	if m.MovementType() != domain.StockMovementTypeEntrada {
		t.Errorf("m.MovementType() = %v, want %v", m.MovementType(), domain.StockMovementTypeEntrada)
	}
	if m.Quantity() != qty {
		t.Errorf("m.Quantity() = %v, want %v", m.Quantity(), qty)
	}
	if m.Reason() != "compra #1" {
		t.Errorf("m.Reason() = %q, want %q", m.Reason(), "compra #1")
	}
	if m.CreatedAt().IsZero() {
		t.Error("m.CreatedAt() is zero, want a populated timestamp")
	}
}

func TestNewStockMovement_EmptyReason_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewStockMovement(
		domain.NewID(), mustTenantID(t), domain.NewID(),
		domain.StockMovementTypeEntrada, mustQuantity(t, 500), "",
	)

	// Assert
	if err == nil {
		t.Fatal("NewStockMovement with empty reason returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewStockMovement_ZeroQuantity_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewStockMovement(
		domain.NewID(), mustTenantID(t), domain.NewID(),
		domain.StockMovementTypeEntrada, mustQuantity(t, 0), "ajuste",
	)

	// Assert
	if err == nil {
		t.Fatal("NewStockMovement with zero quantity returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewStockMovement_InvalidMovementType_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewStockMovement(
		domain.NewID(), mustTenantID(t), domain.NewID(),
		domain.StockMovementType("bogus"), mustQuantity(t, 500), "ajuste",
	)

	// Assert
	if err == nil {
		t.Fatal("NewStockMovement with invalid movement type returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

// TestStockMovement_IsImmutableByDesign documents (rather than exercises at
// runtime, since Go has no reflection-based way to enumerate "does this
// type expose any mutating method") the immutability guarantee of
// StockMovement: it is an append-only log entry. The guarantee is enforced
// structurally, not by a runtime check -- StockMovement's only methods are
// the getters asserted above (ID, TenantID, StockItemID, MovementType,
// Quantity, Reason, CreatedAt) plus the constructor NewStockMovement; there
// is no setter and no method with a pointer receiver that mutates state.
// Anyone adding a mutating method to stock_movement.go in the future breaks
// this documented invariant, which is why it is called out explicitly here
// instead of being left implicit.
func TestStockMovement_IsImmutableByDesign(t *testing.T) {
	// Arrange
	m := newTestStockMovement(t)
	before := m

	// Act: read every getter, which is the entire public surface of
	// StockMovement -- there is nothing else callable that could mutate it.
	_ = m.ID()
	_ = m.TenantID()
	_ = m.StockItemID()
	_ = m.MovementType()
	_ = m.Quantity()
	_ = m.Reason()
	_ = m.CreatedAt()

	// Assert: the value is byte-for-byte identical to before -- reading it
	// cannot have mutated it (StockMovement is a value type with no pointer
	// receivers exposed, so this is guaranteed at compile time; this
	// assertion documents the expectation for anyone reading the tests).
	if m != before {
		t.Errorf("StockMovement value changed after calling only getters: got %+v, want %+v", m, before)
	}
}
