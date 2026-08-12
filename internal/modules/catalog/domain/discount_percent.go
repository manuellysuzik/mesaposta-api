package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// discountBasisScale is DiscountPercent's fixed-point scale: a
// DiscountPercent stores HUNDREDTHS OF A PERCENTAGE POINT (2 decimal
// places), so 12.34% is stored as 1234 and 100% (the maximum) is stored as
// 10000. This is the exact same "store the smallest meaningful unit as an
// integer" convention Money (cents) and Quantity (thousandths of a unit)
// already use in this package, chosen for the same reason: a discount
// like 33.33% cannot be represented exactly by a binary float64 either, so
// DiscountPercent avoids that hazard the same way Money/Quantity do,
// rather than introducing a third, float-based representation into this
// module's value objects.
const discountBasisScale = 10000

// DiscountPercent is a percentage discount in the closed range [0%, 100%],
// expressed as an int64 number of hundredths of a percentage point (see
// discountBasisScale). The zero value (0 hundredths, i.e. 0%) is valid;
// NewDiscountPercent is the only constructor, so a DiscountPercent can
// never fall outside [0, 10000].
type DiscountPercent struct {
	hundredths int64
}

// NewDiscountPercent builds a DiscountPercent from a hundredths-of-a-
// percent value. Values outside [0, 10000] (i.e. outside [0%, 100%]) are
// rejected.
func NewDiscountPercent(hundredths int64) (DiscountPercent, error) {
	if hundredths < 0 || hundredths > discountBasisScale {
		return DiscountPercent{}, errs.New(errs.Invalid, "invalid_discount_percent",
			"discount percent must be between 0 and 100",
			fmt.Errorf("hundredths=%d is outside [0, %d]", hundredths, discountBasisScale))
	}
	return DiscountPercent{hundredths: hundredths}, nil
}

// Hundredths returns d's underlying value, in hundredths of a percentage
// point (e.g. 1234 for 12.34%).
func (d DiscountPercent) Hundredths() int64 {
	return d.hundredths
}
