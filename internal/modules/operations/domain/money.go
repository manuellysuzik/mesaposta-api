package domain

import (
	"fmt"
	"math"
	"math/big"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Money is a monetary amount expressed as an int64 number of CENTAVOS
// (cents), avoiding the rounding hazards float64 would introduce into
// financial math. The zero value (0 cents) is valid; NewMoney is the only
// constructor, so a Money can never represent a negative amount.
type Money struct {
	cents int64
}

// NewMoney builds a Money from a cents value. Negative values are
// rejected.
func NewMoney(cents int64) (Money, error) {
	if cents < 0 {
		return Money{}, errs.New(errs.Invalid, "invalid_money",
			"money must not be negative",
			fmt.Errorf("cents=%d is negative", cents))
	}
	return Money{cents: cents}, nil
}

// Cents returns m's underlying value, in cents.
func (m Money) Cents() int64 {
	return m.cents
}

// Add returns the sum of m and other as a new Money, leaving both m and
// other untouched (Money is an immutable value object).
//
// Add returns an *errs.Error (Kind Invalid) instead of silently wrapping
// around if the sum would overflow int64. Both operands are always
// non-negative (enforced by NewMoney), so overflow can only happen in the
// positive direction.
func (m Money) Add(other Money) (Money, error) {
	if m.cents > math.MaxInt64-other.cents {
		return Money{}, errs.New(errs.Invalid, "money_overflow",
			"money addition overflows",
			fmt.Errorf("%d + %d overflows int64", m.cents, other.cents))
	}
	return Money{cents: m.cents + other.cents}, nil
}

// Sub returns m minus other as a new Money, leaving both m and other
// untouched.
//
// Sub returns an *errs.Error (Kind Invalid) when other exceeds m, instead
// of silently saturating at zero -- see Quantity.Sub's doc comment for why
// that clamping behavior is exactly the bug this method must not repeat.
func (m Money) Sub(other Money) (Money, error) {
	if other.cents > m.cents {
		return Money{}, errs.New(errs.Invalid, "money_underflow",
			"money subtraction would be negative",
			fmt.Errorf("%d - %d would be negative", m.cents, other.cents))
	}
	return Money{cents: m.cents - other.cents}, nil
}

// mulRoundingHalf is the additive term used by Mul to implement half-up
// rounding via integer division: adding half of the divisor (1000) before
// dividing turns a truncating division into a round-to-nearest one for any
// non-negative dividend.
var mulRoundingHalf = big.NewInt(500)

// mulScale is q's fixed-point scale: Quantity stores thousandths of a
// unit, so the raw cents*thousandths product is in thousandths-of-a-cent
// and must be divided back down by 1000 to land on whole cents.
var mulScale = big.NewInt(1000)

// Mul returns the cost of q units priced at m per unit (e.g.
// costPerUnit.Mul(quantity)), rounding the result to the nearest cent
// using half-up rounding. Since both m and q are always non-negative,
// "round half up" and "round half away from zero" coincide, so there is no
// ambiguity about which direction a tie rounds.
//
// q is expressed in thousandths of a unit, so the exact product
// m.cents*q.thousandths is in thousandths-of-a-cent; Mul divides that back
// down to whole cents (see mulScale). The intermediate product is computed
// with math/big rather than raw int64 arithmetic so that a large
// cents/quantity combination can never silently wrap around to a bogus
// (possibly negative) result; Mul panics if the final, rounded amount
// still does not fit in an int64 number of cents, since Money's
// representation is int64 by design and reaching that magnitude signals a
// genuine invariant violation rather than a recoverable input error (see
// Quantity/Money.Add's doc comments for the same "never wrap around
// silently" principle applied to addition).
func (m Money) Mul(q Quantity) Money {
	product := new(big.Int).Mul(big.NewInt(m.cents), big.NewInt(q.thousandths))
	product.Add(product, mulRoundingHalf)
	rounded := new(big.Int).Div(product, mulScale)

	if !rounded.IsInt64() {
		panic(fmt.Sprintf("domain: Money.Mul: result %s overflows int64 cents", rounded.String()))
	}
	return Money{cents: rounded.Int64()}
}
