package domain

import (
	"fmt"
	"math"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Quantity is a stock quantity (kg, l, un, ...) expressed as an int64
// number of THOUSANDTHS of a unit, giving exact fixed-point precision up
// to 3 decimal places -- 1.333 kg is stored as 1333 -- without the
// rounding hazards float64 would introduce into stock and cost math. The
// zero value (0 thousandths) is a valid quantity ("no stock"); NewQuantity
// is the only constructor, so a Quantity can never represent negative
// stock.
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
//
// Add returns an *errs.Error (Kind Invalid) instead of silently wrapping
// around if the sum would overflow int64. Both operands are always
// non-negative (enforced by NewQuantity), so overflow can only happen in
// the positive direction.
func (q Quantity) Add(other Quantity) (Quantity, error) {
	if q.thousandths > math.MaxInt64-other.thousandths {
		return Quantity{}, errs.New(errs.Invalid, "quantity_overflow",
			"quantity addition overflows",
			fmt.Errorf("%d + %d overflows int64", q.thousandths, other.thousandths))
	}
	return Quantity{thousandths: q.thousandths + other.thousandths}, nil
}

// Sub returns q minus other as a new Quantity, leaving both q and other
// untouched.
//
// Unlike a naive implementation that clamps a would-be-negative result to
// zero via something like Math.max(0, ...), Sub returns an *errs.Error
// (Kind Invalid) when other exceeds q instead: silently saturating at zero
// would hide a real bug (over-consuming stock that was never actually
// there) behind a plausible-looking zero value.
func (q Quantity) Sub(other Quantity) (Quantity, error) {
	if other.thousandths > q.thousandths {
		return Quantity{}, errs.New(errs.Invalid, "quantity_underflow",
			"quantity subtraction would be negative",
			fmt.Errorf("%d - %d would be negative", q.thousandths, other.thousandths))
	}
	return Quantity{thousandths: q.thousandths - other.thousandths}, nil
}
