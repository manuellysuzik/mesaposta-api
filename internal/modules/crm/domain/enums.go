package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// LoyaltyEntryType identifies whether a LoyaltyEntry credited or redeemed
// points, following the same closed-set pattern as
// internal/modules/operations/domain.StockMovementType: a string-based
// type whose zero value ("") is deliberately invalid, a Valid method that
// reports membership in the known set, and a Parse<Name> constructor that
// rejects any unknown (including empty) value with an *errs.Error of Kind
// errs.Invalid instead of letting it flow through as a silent, unvalidated
// default.
type LoyaltyEntryType string

const (
	// LoyaltyEntryTypeCredito is an inbound ledger entry: points earned/
	// credited to a customer.
	LoyaltyEntryTypeCredito LoyaltyEntryType = "credito"
	// LoyaltyEntryTypeResgate is an outbound ledger entry: points redeemed/
	// spent by a customer.
	LoyaltyEntryTypeResgate LoyaltyEntryType = "resgate"
)

// Valid reports whether t is one of the closed set of known loyalty entry
// types.
func (t LoyaltyEntryType) Valid() bool {
	switch t {
	case LoyaltyEntryTypeCredito, LoyaltyEntryTypeResgate:
		return true
	default:
		return false
	}
}

// ParseLoyaltyEntryType validates s against the closed set of known loyalty
// entry types.
func ParseLoyaltyEntryType(s string) (LoyaltyEntryType, error) {
	t := LoyaltyEntryType(s)
	if !t.Valid() {
		return "", errs.New(errs.Invalid, "invalid_loyalty_entry_type", "unknown loyalty entry type",
			fmt.Errorf("loyalty entry type %q is not one of the known values", s))
	}
	return t, nil
}
