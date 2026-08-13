package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Quantity is a whole-number count of units of a product ordered (e.g. "2 x
// X-Burguer"), expressed as an int64. Unlike
// internal/modules/operations/domain.Quantity and
// internal/modules/catalog/domain.Quantity (both THOUSANDTHS of a unit, for
// fractional stock like kg), an order line is always a whole number of
// discrete items -- there is no "1.5 X-Burguer" -- so this module's own
// Quantity has no fixed-point scale at all: it is a plain, exact integer
// count.
//
// NewQuantity is the only constructor and requires a STRICTLY POSITIVE
// count: an order line for zero (or fewer) of a product makes no sense --
// see NewOrderItem, the sole caller.
type Quantity struct {
	count int64
}

// NewQuantity builds a Quantity from a plain whole-unit count. Zero and
// negative values are both rejected.
func NewQuantity(count int64) (Quantity, error) {
	if count <= 0 {
		return Quantity{}, errs.New(errs.Invalid, "invalid_quantity",
			"quantity must be greater than zero",
			fmt.Errorf("count=%d is not positive", count))
	}
	return Quantity{count: count}, nil
}

// Count returns q's underlying value.
func (q Quantity) Count() int64 {
	return q.count
}
