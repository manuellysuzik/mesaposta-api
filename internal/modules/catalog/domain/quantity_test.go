package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
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
	// Arrange & Act
	q, err := domain.NewQuantity(-1)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if q.Thousandths() != 0 {
		t.Errorf("NewQuantity(-1) returned non-zero Quantity on error")
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
}

func TestQuantity_Sub_ResultWouldBeNegative_ReturnsError(t *testing.T) {
	// Arrange
	a := mustQuantity(t, 1000)
	b := mustQuantity(t, 2000)

	// Act
	diff, err := a.Sub(b)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if diff.Thousandths() != 0 {
		t.Errorf("Sub() on error = %d, want 0", diff.Thousandths())
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
