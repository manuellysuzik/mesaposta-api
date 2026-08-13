package domain

import (
	"fmt"
	"math"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Money is a monetary amount expressed as an int64 number of CENTAVOS
// (cents), avoiding the rounding hazards float64 would introduce into
// financial math -- the same representation
// internal/modules/operations/domain.Money and
// internal/modules/catalog/domain.Money already establish. The zero value
// (0 cents) is valid; NewMoney is the only constructor, so a Money can
// never represent a negative amount.
//
// This module deliberately does NOT share operations'/catalog's own Money
// type: architecture rule R3 forbids one module's domain/ from importing
// another module's domain/ at all, so each module owns its own copy of
// this small, self-contained value object.
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

// MulInt returns m multiplied by count -- a non-negative WHOLE number of
// units (e.g. unitPrice.MulInt(quantity.Count()) to compute an OrderItem's
// line total). Unlike operations'/catalog's own Money.Mul (which multiplies
// by a FRACTIONAL Quantity and therefore needs half-up rounding),
// ordering's own Quantity is always a whole count (see quantity.go), so
// this is exact integer multiplication with no rounding at all -- but
// still guarded against int64 overflow, mirroring Add's own "never wrap
// around silently" principle, rather than trusting a raw `m.cents * count`
// expression not to overflow.
func (m Money) MulInt(count int64) (Money, error) {
	if count < 0 {
		return Money{}, errs.New(errs.Invalid, "invalid_money_multiplier",
			"money multiplier must not be negative",
			fmt.Errorf("count=%d is negative", count))
	}
	if count != 0 && m.cents > math.MaxInt64/count {
		return Money{}, errs.New(errs.Invalid, "money_overflow",
			"money multiplication overflows",
			fmt.Errorf("%d * %d overflows int64", m.cents, count))
	}
	return Money{cents: m.cents * count}, nil
}
