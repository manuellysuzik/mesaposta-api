package domain

import (
	"fmt"
	"math"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Quantity is an ingredient quantity (kg, l, un, ...) expressed as an
// int64 number of THOUSANDTHS of a unit, giving exact fixed-point
// precision up to 3 decimal places -- 1.333 kg is stored as 1333 --
// without the rounding hazards float64 would introduce. The zero value (0
// thousandths) is a valid quantity; NewQuantity is the only constructor,
// so a Quantity can never represent a negative amount.
//
// This mirrors internal/modules/operations/domain.Quantity exactly -- see
// doc.go for why this module owns its own copy instead of importing
// operations/domain (architecture rule R3).
type Quantity struct {
	thousandths int64
}

// NewQuantity builds a Quantity from a thousandths-of-a-unit value.
// Negative values are rejected.
func NewQuantity(thousandths int64) (Quantity, error) {
	if thousandths < 0 {
		return Quantity{}, errs.New(errs.Invalid, "invalid_quantity",
			"quantity must not be negative",
			fmt.Errorf("thousandths=%d is negative", thousandths))
	}
	return Quantity{thousandths: thousandths}, nil
}

// Thousandths returns q's underlying value, in thousandths of a unit.
func (q Quantity) Thousandths() int64 {
	return q.thousandths
}

// Add returns the sum of q and other as a new Quantity, leaving both q and
// other untouched (Quantity is an immutable value object).
func (q Quantity) Add(other Quantity) (Quantity, error) {
	if q.thousandths > math.MaxInt64-other.thousandths {
		return Quantity{}, errs.New(errs.Invalid, "quantity_overflow",
			"quantity addition overflows",
			fmt.Errorf("%d + %d overflows int64", q.thousandths, other.thousandths))
	}
	return Quantity{thousandths: q.thousandths + other.thousandths}, nil
}

// Sub returns q minus other as a new Quantity, leaving both q and other
// untouched. It returns an *errs.Error (Kind Invalid) when other exceeds
// q instead of silently saturating at zero.
func (q Quantity) Sub(other Quantity) (Quantity, error) {
	if other.thousandths > q.thousandths {
		return Quantity{}, errs.New(errs.Invalid, "quantity_underflow",
			"quantity subtraction would be negative",
			fmt.Errorf("%d - %d would be negative", q.thousandths, other.thousandths))
	}
	return Quantity{thousandths: q.thousandths - other.thousandths}, nil
}
