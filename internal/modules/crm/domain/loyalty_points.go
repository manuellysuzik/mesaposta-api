package domain

import (
	"fmt"
	"math"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// LoyaltyPoints is a whole-number count of loyalty points, expressed as an
// int64 -- never a float, per this module's spec, so a point balance can
// never drift due to floating-point rounding. The zero value (0 points) is
// valid ("no points earned yet"); NewLoyaltyPoints is the only constructor,
// so a LoyaltyPoints can never represent a negative balance.
type LoyaltyPoints struct {
	points int64
}

// NewLoyaltyPoints builds a LoyaltyPoints from a plain int64 point count.
// Negative values are rejected.
func NewLoyaltyPoints(points int64) (LoyaltyPoints, error) {
	if points < 0 {
		return LoyaltyPoints{}, errs.New(errs.Invalid, "invalid_loyalty_points",
			"loyalty points must not be negative",
			fmt.Errorf("points=%d is negative", points))
	}
	return LoyaltyPoints{points: points}, nil
}

// Points returns p's underlying value.
func (p LoyaltyPoints) Points() int64 {
	return p.points
}

// Add returns the sum of p and other as a new LoyaltyPoints, leaving both p
// and other untouched (LoyaltyPoints is an immutable value object).
//
// Add returns an *errs.Error (Kind Invalid) instead of silently wrapping
// around if the sum would overflow int64. Both operands are always
// non-negative (enforced by NewLoyaltyPoints), so overflow can only happen
// in the positive direction.
func (p LoyaltyPoints) Add(other LoyaltyPoints) (LoyaltyPoints, error) {
	if p.points > math.MaxInt64-other.points {
		return LoyaltyPoints{}, errs.New(errs.Invalid, "loyalty_points_overflow",
			"loyalty points addition overflows",
			fmt.Errorf("%d + %d overflows int64", p.points, other.points))
	}
	return LoyaltyPoints{points: p.points + other.points}, nil
}

// Sub returns p minus other as a new LoyaltyPoints, leaving both p and
// other untouched.
//
// Unlike a naive implementation that clamps a would-be-negative result to
// zero, Sub returns an *errs.Error (Kind Invalid) when other exceeds p
// instead: silently saturating at zero would hide a real bug (redeeming
// more points than a customer ever had) behind a plausible-looking zero
// value. Customer.RedeemPoints is the sole caller that turns this Invalid
// classification into the Conflict this module's acceptance criterion
// requires -- see customer.go's own doc comment.
func (p LoyaltyPoints) Sub(other LoyaltyPoints) (LoyaltyPoints, error) {
	if other.points > p.points {
		return LoyaltyPoints{}, errs.New(errs.Invalid, "loyalty_points_underflow",
			"loyalty points subtraction would be negative",
			fmt.Errorf("%d - %d would be negative", p.points, other.points))
	}
	return LoyaltyPoints{points: p.points - other.points}, nil
}
