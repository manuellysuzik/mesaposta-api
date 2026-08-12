package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewMoney_NonNegative_Succeeds(t *testing.T) {
	// Arrange
	tests := []int64{0, 1, 100, math.MaxInt64}

	for _, cents := range tests {
		// Act
		m, err := domain.NewMoney(cents)

		// Assert
		if err != nil {
			t.Fatalf("NewMoney(%d) returned unexpected error: %v", cents, err)
		}
		if m.Cents() != cents {
			t.Errorf("NewMoney(%d).Cents() = %d, want %d", cents, m.Cents(), cents)
		}
	}
}

func TestNewMoney_Negative_ReturnsInvalidKindError(t *testing.T) {
	// Arrange & Act
	m, err := domain.NewMoney(-1)

	// Assert
	if err == nil {
		t.Fatal("NewMoney(-1) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if m.Cents() != 0 {
		t.Errorf("NewMoney(-1) returned non-zero Money on error")
	}
}

func TestMoney_Add_SumsCents(t *testing.T) {
	// Arrange
	a := mustMoney(t, 150)
	b := mustMoney(t, 333)

	// Act
	sum, err := a.Add(b)

	// Assert
	if err != nil {
		t.Fatalf("Add returned unexpected error: %v", err)
	}
	if sum.Cents() != 483 {
		t.Errorf("Add() = %d, want %d", sum.Cents(), 483)
	}
}

func TestMoney_Add_OverflowingInt64_ReturnsError(t *testing.T) {
	// Arrange
	a := mustMoney(t, math.MaxInt64)
	b := mustMoney(t, 1)

	// Act
	_, err := a.Add(b)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestMoney_Sub_ResultWouldBeNegative_ReturnsError(t *testing.T) {
	// Arrange
	a := mustMoney(t, 100)
	b := mustMoney(t, 200)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if diff.Cents() != 0 {
		t.Errorf("Sub() on error = %d, want 0", diff.Cents())
	}
}

func TestMoney_Mul_KnownValue_RoundsHalfUp(t *testing.T) {
	// Arrange: R$ 2,50/kg (250 cents) x 1,333 kg (1333 thousandths) must
	// round to R$ 3,33 (333 cents).
	costPerUnit := mustMoney(t, 250)
	quantity := mustQuantity(t, 1333)

	// Act
	total := costPerUnit.Mul(quantity)

	// Assert
	if total.Cents() != 333 {
		t.Errorf("Mul() = %d cents, want %d cents", total.Cents(), 333)
	}
}

func TestMoney_ApplyDiscount_NoDiscount_ReturnsSameAmount(t *testing.T) {
	// Arrange
	price := mustMoney(t, 1000)
	discount := mustDiscountPercent(t, 0)

	// Act
	got := price.ApplyDiscount(discount)

	// Assert
	if got.Cents() != 1000 {
		t.Errorf("ApplyDiscount(0%%) = %d, want %d", got.Cents(), 1000)
	}
}

func TestMoney_ApplyDiscount_FullDiscount_ReturnsZero(t *testing.T) {
	// Arrange
	price := mustMoney(t, 1000)
	discount := mustDiscountPercent(t, 10000) // 100%

	// Act
	got := price.ApplyDiscount(discount)

	// Assert
	if got.Cents() != 0 {
		t.Errorf("ApplyDiscount(100%%) = %d, want 0", got.Cents())
	}
}

// TestMoney_ApplyDiscount_HalfDiscount_RoundsHalfUp pins down a fractional
// case: R$ 10,01 (1001 cents) with a 50% discount = R$ 5,005, which must
// round UP to 501 cents under half-up rounding.
func TestMoney_ApplyDiscount_HalfDiscount_RoundsHalfUp(t *testing.T) {
	// Arrange
	price := mustMoney(t, 1001)
	discount := mustDiscountPercent(t, 5000) // 50%

	// Act
	got := price.ApplyDiscount(discount)

	// Assert
	if got.Cents() != 501 {
		t.Errorf("ApplyDiscount(50%%) = %d, want %d", got.Cents(), 501)
	}
}

func TestMoney_ApplyDiscount_PartialDiscount_ComputesRemainder(t *testing.T) {
	// Arrange: R$ 20,00 (2000 cents) with a 25% discount = R$ 15,00 (1500 cents).
	price := mustMoney(t, 2000)
	discount := mustDiscountPercent(t, 2500) // 25%

	// Act
	got := price.ApplyDiscount(discount)

	// Assert
	if got.Cents() != 1500 {
		t.Errorf("ApplyDiscount(25%%) = %d, want %d", got.Cents(), 1500)
	}
}

// mustMoney is a test helper that builds a Money from a known-valid cents
// value, failing the test immediately if construction errors.
func mustMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}
