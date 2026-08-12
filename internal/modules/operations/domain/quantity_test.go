package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewQuantity_NonNegative_Succeeds(t *testing.T) {
	// Arrange
	tests := []int64{0, 1, 1000, math.MaxInt64}

	for _, thousandths := range tests {
		// Act
		q, err := domain.NewQuantity(thousandths)

		// Assert
		if err != nil {
			t.Fatalf("NewQuantity(%d) returned unexpected error: %v", thousandths, err)
		}
		if q.Thousandths() != thousandths {
			t.Errorf("NewQuantity(%d).Thousandths() = %d, want %d", thousandths, q.Thousandths(), thousandths)
		}
	}
}

func TestNewQuantity_Negative_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	tests := []int64{-1, -1000, math.MinInt64}

	for _, thousandths := range tests {
		// Act
		q, err := domain.NewQuantity(thousandths)

		// Assert
		if err == nil {
			t.Fatalf("NewQuantity(%d) returned nil error, want an error", thousandths)
		}
		if !errs.IsKind(err, errs.Invalid) {
			t.Errorf("NewQuantity(%d) error kind = %v, want %v", thousandths, errs.KindOf(err), errs.Invalid)
		}
		if q.Thousandths() != 0 {
			t.Errorf("NewQuantity(%d) returned non-zero Quantity on error", thousandths)
		}
	}
}

func TestQuantity_Add_SumsThousandths(t *testing.T) {
	// Arrange
	a := mustQuantity(t, 1500)
	b := mustQuantity(t, 2333)

	// Act
	sum, err := a.Add(b)

	// Assert
	if err != nil {
		t.Fatalf("Add returned unexpected error: %v", err)
	}
	if sum.Thousandths() != 3833 {
		t.Errorf("Add() = %d, want %d", sum.Thousandths(), 3833)
	}
	// a must not be mutated -- Quantity is an immutable value object.
	if a.Thousandths() != 1500 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Thousandths(), 1500)
	}
}

func TestQuantity_Add_OverflowingInt64_ReturnsError(t *testing.T) {
	// Arrange
	a := mustQuantity(t, math.MaxInt64)
	b := mustQuantity(t, 1)

	// Act
	sum, err := a.Add(b)

	// Assert
	if err == nil {
		t.Fatal("Add() at int64 boundary returned nil error, want an overflow error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Add() overflow error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if sum.Thousandths() != 0 {
		t.Errorf("Add() overflow returned non-zero Quantity: %d", sum.Thousandths())
	}
	if a.Thousandths() != math.MaxInt64 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Thousandths(), int64(math.MaxInt64))
	}
}

func TestQuantity_Sub_ResultNonNegative_Succeeds(t *testing.T) {
	// Arrange
	a := mustQuantity(t, 5000)
	b := mustQuantity(t, 2000)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if err != nil {
		t.Fatalf("Sub returned unexpected error: %v", err)
	}
	if diff.Thousandths() != 3000 {
		t.Errorf("Sub() = %d, want %d", diff.Thousandths(), 3000)
	}
}

func TestQuantity_Sub_ResultWouldBeNegative_ReturnsErrorAndLeavesOriginalIntact(t *testing.T) {
	// Arrange
	a := mustQuantity(t, 1000)
	b := mustQuantity(t, 2000)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if err == nil {
		t.Fatal("Sub() with negative result returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Sub() error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	// Sub must NOT saturate at zero -- the returned value on error must be
	// the (unusable) zero Quantity, and the original operand `a` must be
	// left completely untouched.
	if diff.Thousandths() != 0 {
		t.Errorf("Sub() on error returned non-zero Quantity: %d", diff.Thousandths())
	}
	if a.Thousandths() != 1000 {
		t.Errorf("original operand a mutated: got %d, want %d", a.Thousandths(), 1000)
	}
}

func TestQuantity_Sub_EqualOperands_ReturnsZero(t *testing.T) {
	// Arrange
	a := mustQuantity(t, 2500)
	b := mustQuantity(t, 2500)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if err != nil {
		t.Fatalf("Sub returned unexpected error: %v", err)
	}
	if diff.Thousandths() != 0 {
		t.Errorf("Sub() of equal operands = %d, want 0", diff.Thousandths())
	}
}

// mustQuantity is a test helper that builds a Quantity from a known-valid
// thousandths value, failing the test immediately if construction errors.
func mustQuantity(t *testing.T, thousandths int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(thousandths)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", thousandths, err)
	}
	return q
}
