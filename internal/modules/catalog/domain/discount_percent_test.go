package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewDiscountPercent_WithinRange_Succeeds(t *testing.T) {
	// Arrange
	tests := []int64{0, 1, 1234, 9999, 10000}

	for _, hundredths := range tests {
		// Act
		d, err := domain.NewDiscountPercent(hundredths)

		// Assert
		if err != nil {
			t.Fatalf("NewDiscountPercent(%d) returned unexpected error: %v", hundredths, err)
		}
		if d.Hundredths() != hundredths {
			t.Errorf("NewDiscountPercent(%d).Hundredths() = %d, want %d", hundredths, d.Hundredths(), hundredths)
		}
	}
}

func TestNewDiscountPercent_OutOfRange_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	tests := []int64{-1, 10001, -100, 100000}

	for _, hundredths := range tests {
		// Act
		d, err := domain.NewDiscountPercent(hundredths)

		// Assert
		if err == nil {
			t.Fatalf("NewDiscountPercent(%d) returned nil error, want an error", hundredths)
		}
		if !errs.IsKind(err, errs.Invalid) {
			t.Errorf("NewDiscountPercent(%d) error kind = %v, want %v", hundredths, errs.KindOf(err), errs.Invalid)
		}
		if d.Hundredths() != 0 {
			t.Errorf("NewDiscountPercent(%d) returned non-zero DiscountPercent on error", hundredths)
		}
	}
}

// mustDiscountPercent is a test helper that builds a DiscountPercent from
// a known-valid hundredths value, failing the test immediately if
// construction errors.
func mustDiscountPercent(t *testing.T, hundredths int64) domain.DiscountPercent {
	t.Helper()
	d, err := domain.NewDiscountPercent(hundredths)
	if err != nil {
		t.Fatalf("NewDiscountPercent(%d): %v", hundredths, err)
	}
	return d
}
