package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
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
	// Arrange
	tests := []int64{-1, -100, math.MinInt64}

	for _, cents := range tests {
		// Act
		m, err := domain.NewMoney(cents)

		// Assert
		if err == nil {
			t.Fatalf("NewMoney(%d) returned nil error, want an error", cents)
		}
		if !errs.IsKind(err, errs.Invalid) {
			t.Errorf("NewMoney(%d) error kind = %v, want %v", cents, errs.KindOf(err), errs.Invalid)
		}
		if m.Cents() != 0 {
			t.Errorf("NewMoney(%d) returned non-zero Money on error", cents)
		}
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
	if a.Cents() != 150 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Cents(), 150)
	}
}

func TestMoney_Add_OverflowingInt64_ReturnsError(t *testing.T) {
	// Arrange
	a := mustMoney(t, math.MaxInt64)
	b := mustMoney(t, 1)

	// Act
	sum, err := a.Add(b)

	// Assert
	if err == nil {
		t.Fatal("Add() at int64 boundary returned nil error, want an overflow error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Add() overflow error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if sum.Cents() != 0 {
		t.Errorf("Add() overflow returned non-zero Money: %d", sum.Cents())
	}
	if a.Cents() != math.MaxInt64 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Cents(), int64(math.MaxInt64))
	}
}

func TestMoney_Sub_ResultNonNegative_Succeeds(t *testing.T) {
	// Arrange
	a := mustMoney(t, 500)
	b := mustMoney(t, 200)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if err != nil {
		t.Fatalf("Sub returned unexpected error: %v", err)
	}
	if diff.Cents() != 300 {
		t.Errorf("Sub() = %d, want %d", diff.Cents(), 300)
	}
}

func TestMoney_Sub_ResultWouldBeNegative_ReturnsErrorAndLeavesOriginalIntact(t *testing.T) {
	// Arrange
	a := mustMoney(t, 100)
	b := mustMoney(t, 200)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if err == nil {
		t.Fatal("Sub() with negative result returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Sub() error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	// Sub must NOT saturate at zero -- must error, and leave `a` untouched.
	if diff.Cents() != 0 {
		t.Errorf("Sub() on error returned non-zero Money: %d", diff.Cents())
	}
	if a.Cents() != 100 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Cents(), 100)
	}
}

// TestMoney_Mul_KnownValue_RoundsHalfUp pins down the exact worked example
// from the task spec: R$ 2,50/kg (250 cents) x 1,333 kg (1333 thousandths)
// must round to R$ 3,33 (333 cents).
//
// Exact product: 250 * 1333 = 333250 (thousandths-of-a-cent).
// Half-up rounding: (333250 + 500) / 1000 = 333750 / 1000 = 333 (integer
// division truncates the .75 remainder away, which is exactly "round to
// nearest, ties away from zero" for this magnitude).
func TestMoney_Mul_KnownValue_RoundsHalfUp(t *testing.T) {
	// Arrange
	costPerUnit := mustMoney(t, 250)
	quantity := mustQuantity(t, 1333)

	// Act
	total := costPerUnit.Mul(quantity)

	// Assert
	if total.Cents() != 333 {
		t.Errorf("Mul() = %d cents, want %d cents (R$ 3,33)", total.Cents(), 333)
	}
}

// TestMoney_Mul_ExactMultiple_NoRoundingNeeded verifies Mul against a case
// with no fractional remainder: R$ 2,00 (200 cents) x 1,000 kg (1000
// thousandths) = R$ 2,00 exactly (200 cents), no rounding involved.
func TestMoney_Mul_ExactMultiple_NoRoundingNeeded(t *testing.T) {
	// Arrange
	costPerUnit := mustMoney(t, 200)
	quantity := mustQuantity(t, 1000)

	// Act
	total := costPerUnit.Mul(quantity)

	// Assert
	if total.Cents() != 200 {
		t.Errorf("Mul() = %d cents, want %d cents", total.Cents(), 200)
	}
}

// TestMoney_Mul_ExactHalfCent_RoundsUp pins down the boundary case: 1 cent
// x 0,5 units (500 thousandths) = 0,5 cents exactly, which must round UP to
// 1 cent under half-up rounding (not down to 0, and not banker's rounding
// to even).
//
// Exact product: 1 * 500 = 500. (500 + 500) / 1000 = 1000 / 1000 = 1.
func TestMoney_Mul_ExactHalfCent_RoundsUp(t *testing.T) {
	// Arrange
	costPerUnit := mustMoney(t, 1)
	quantity := mustQuantity(t, 500)

	// Act
	total := costPerUnit.Mul(quantity)

	// Assert
	if total.Cents() != 1 {
		t.Errorf("Mul() = %d cents, want %d cents", total.Cents(), 1)
	}
}

// TestMoney_Mul_ZeroQuantity_IsZero verifies the degenerate case: any price
// times zero quantity is zero.
func TestMoney_Mul_ZeroQuantity_IsZero(t *testing.T) {
	// Arrange
	costPerUnit := mustMoney(t, 999)
	quantity := mustQuantity(t, 0)

	// Act
	total := costPerUnit.Mul(quantity)

	// Assert
	if total.Cents() != 0 {
		t.Errorf("Mul() with zero quantity = %d cents, want 0", total.Cents())
	}
}

// TestMoney_Mul_HugeOperands_PanicsInsteadOfWrappingAround verifies that a
// cents/quantity combination whose rounded product cannot fit in an int64
// number of cents panics rather than silently wrapping around to a bogus
// (possibly negative) Money value -- Money's cents field is int64 by
// design, so this magnitude is a genuine invariant violation, not a
// recoverable input error.
func TestMoney_Mul_HugeOperands_PanicsInsteadOfWrappingAround(t *testing.T) {
	// Arrange
	costPerUnit := mustMoney(t, math.MaxInt64)
	quantity := mustQuantity(t, math.MaxInt64)

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Mul() with huge operands did not panic, want a panic instead of silent int64 wraparound")
		}
	}()
	_ = costPerUnit.Mul(quantity)
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
