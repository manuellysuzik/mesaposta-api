package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// LoyaltyEntry is a single, immutable ledger entry recording one change to
// a Customer's loyalty point balance -- a credit (points earned) or a
// redemption (points spent). It is a CHILD ENTITY of the Customer aggregate
// (see customer.go's own doc comment for why that boundary was chosen
// deliberately), not an aggregate root of its own: a LoyaltyEntry is only
// ever meaningful in the context of the Customer whose balance it recorded
// a change to, and is always created and persisted together with that same
// Customer mutation, inside a single UnitOfWork.Do call (see
// ../application/credit_loyalty_points.go and redeem_loyalty_points.go).
//
// It is append-only by design, mirroring
// internal/modules/operations/domain.StockMovement: once constructed, a
// LoyaltyEntry exposes only getters and can never be mutated. There is no
// setter and no pointer-receiver method anywhere in this file; that
// omission is the enforcement mechanism, not a runtime check.
type LoyaltyEntry struct {
	id         ID
	tenantID   tenant.TenantID
	customerID ID
	entryType  LoyaltyEntryType
	points     LoyaltyPoints
	reason     string
	createdAt  time.Time
}

// NewLoyaltyEntry builds a LoyaltyEntry, validating that customerID is not
// the zero ID, entryType is one of the known loyalty entry types, points is
// non-zero (a zero-point entry records nothing and would be meaningless
// noise in the ledger), and reason is non-empty.
func NewLoyaltyEntry(
	id ID,
	tenantID tenant.TenantID,
	customerID ID,
	entryType LoyaltyEntryType,
	points LoyaltyPoints,
	reason string,
) (LoyaltyEntry, error) {
	if customerID.IsZero() {
		return LoyaltyEntry{}, errs.New(errs.Invalid, "invalid_loyalty_entry_customer_id",
			"loyalty entry customer ID must not be zero", nil)
	}
	if !entryType.Valid() {
		return LoyaltyEntry{}, errs.New(errs.Invalid, "invalid_loyalty_entry_type",
			"loyalty entry type is not one of the known values", nil)
	}
	if points.Points() == 0 {
		return LoyaltyEntry{}, errs.New(errs.Invalid, "invalid_loyalty_entry_points",
			"loyalty entry points must not be zero", nil)
	}
	if reason == "" {
		return LoyaltyEntry{}, errs.New(errs.Invalid, "invalid_loyalty_entry_reason",
			"loyalty entry reason must not be empty", nil)
	}

	return LoyaltyEntry{
		id:         id,
		tenantID:   tenantID,
		customerID: customerID,
		entryType:  entryType,
		points:     points,
		reason:     reason,
		createdAt:  time.Now().UTC(),
	}, nil
}

// ID returns the loyalty entry's identity.
func (e LoyaltyEntry) ID() ID { return e.id }

// TenantID returns the tenant the loyalty entry belongs to.
func (e LoyaltyEntry) TenantID() tenant.TenantID { return e.tenantID }

// CustomerID returns the ID of the Customer this entry belongs to.
func (e LoyaltyEntry) CustomerID() ID { return e.customerID }

// EntryType returns whether this entry is a credit or a redemption.
func (e LoyaltyEntry) EntryType() LoyaltyEntryType { return e.entryType }

// Points returns the (always non-zero) number of points this entry moved.
func (e LoyaltyEntry) Points() LoyaltyPoints { return e.points }

// Reason returns the human-readable reason recorded for this entry.
func (e LoyaltyEntry) Reason() string { return e.reason }

// CreatedAt returns the timestamp this entry was recorded at.
func (e LoyaltyEntry) CreatedAt() time.Time { return e.createdAt }
