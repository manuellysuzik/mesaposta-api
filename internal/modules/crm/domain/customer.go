package domain

import (
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Customer is a tenant's customer, together with their loyalty program
// balance. It is an aggregate root guarded by an optimistic-concurrency
// Version, incremented on every successful mutation (CreditPoints,
// RedeemPoints).
//
// Aggregate boundary decision -- LoyaltyEntry lives INSIDE Customer, not as
// its own aggregate: the loyalty ledger's core invariant ("a customer can
// never redeem more points than their current balance") can only be
// guaranteed if Customer.loyaltyPoints and the LoyaltyEntry recording that
// change are mutated together, inside the exact same transaction. Had
// LoyaltyEntry been a separate aggregate, two concurrent redemptions could
// each read the same (correct, pre-redemption) balance, each append their
// own entry, and only afterwards discover -- too late -- that together they
// overdrew the balance: a classic race condition an aggregate boundary
// exists specifically to prevent. Nesting LoyaltyEntry inside Customer (and
// requiring every application-layer use case to load-mutate-persist the
// whole Customer, including the optimistic-concurrency Version below,
// within a single UnitOfWork.Do call -- see ../application/
// credit_loyalty_points.go and redeem_loyalty_points.go) is what makes that
// race impossible instead of merely unlikely.
type Customer struct {
	id            ID
	tenantID      tenant.TenantID
	name          string
	email         *string
	phone         *string
	loyaltyPoints LoyaltyPoints
	version       int64
	createdAt     time.Time
	updatedAt     time.Time
}

// initialCustomerVersion is the Version a freshly constructed Customer
// starts at, before any mutation.
const initialCustomerVersion = 1

// NewCustomer builds a Customer, validating that name is non-empty, and
// that email/phone -- both optional -- are not present-but-blank (a *string
// pointing at "" is rejected outright, rather than silently treated as "no
// email"/"no phone": a caller that meant "no email" should pass nil, not an
// empty string, so this guards against that ambiguity leaking into
// persisted data). email and phone are otherwise stored verbatim: this
// phase does not validate email/phone FORMAT (e.g. a proper email regex) --
// see this package's doc comment and the module's final report for that
// deliberately deferred scope decision. Uniqueness of email/phone PER
// TENANT is enforced by a database constraint (see
// internal/platform/db/migrations/0006_crm.sql), not by this constructor:
// NewCustomer has no way to check other rows without I/O, which domain/
// must never perform (architecture rule R1).
//
// The new customer always starts out with a zero loyalty point balance, at
// Version 1.
func NewCustomer(id ID, tenantID tenant.TenantID, name string, email *string, phone *string) (Customer, error) {
	if strings.TrimSpace(name) == "" {
		return Customer{}, errs.New(errs.Invalid, "invalid_customer_name",
			"customer name must not be empty", nil)
	}
	if email != nil && strings.TrimSpace(*email) == "" {
		return Customer{}, errs.New(errs.Invalid, "invalid_customer_email",
			"customer email must not be blank when provided", nil)
	}
	if phone != nil && strings.TrimSpace(*phone) == "" {
		return Customer{}, errs.New(errs.Invalid, "invalid_customer_phone",
			"customer phone must not be blank when provided", nil)
	}

	now := time.Now().UTC()
	return Customer{
		id:            id,
		tenantID:      tenantID,
		name:          name,
		email:         email,
		phone:         phone,
		loyaltyPoints: LoyaltyPoints{},
		version:       initialCustomerVersion,
		createdAt:     now,
		updatedAt:     now,
	}, nil
}

// ID returns the customer's identity.
func (c Customer) ID() ID { return c.id }

// TenantID returns the tenant the customer belongs to.
func (c Customer) TenantID() tenant.TenantID { return c.tenantID }

// Name returns the customer's display name.
func (c Customer) Name() string { return c.name }

// Email returns the customer's email address, or nil if none is set.
func (c Customer) Email() *string { return c.email }

// Phone returns the customer's phone number, or nil if none is set.
func (c Customer) Phone() *string { return c.phone }

// LoyaltyPoints returns the customer's current loyalty point balance.
func (c Customer) LoyaltyPoints() LoyaltyPoints { return c.loyaltyPoints }

// Version returns the customer's optimistic-concurrency version, starting
// at 1 and incrementing by exactly 1 on every successful mutation.
func (c Customer) Version() int64 { return c.version }

// CreatedAt returns the timestamp the customer was created at.
func (c Customer) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the timestamp of the customer's last successful
// mutation (or its construction time, if never mutated).
func (c Customer) UpdatedAt() time.Time { return c.updatedAt }

// CreditPoints adds points to the customer's loyalty point balance (e.g.
// points earned from a completed order). points must be non-zero.
//
// CreditPoints returns an *errs.Error (Kind Invalid, propagated from
// LoyaltyPoints.Add) if the resulting balance would overflow -- the
// aggregate is left untouched in that case. This mirrors
// internal/modules/operations/domain.StockItem.Receive exactly.
func (c *Customer) CreditPoints(points LoyaltyPoints) error {
	if points.Points() == 0 {
		return errs.New(errs.Invalid, "invalid_credit_points",
			"credit points must not be zero", nil)
	}

	sum, err := c.loyaltyPoints.Add(points)
	if err != nil {
		return err
	}

	c.loyaltyPoints = sum
	c.version++
	c.updatedAt = time.Now().UTC()
	return nil
}

// RedeemPoints removes points from the customer's loyalty point balance
// (e.g. points spent on a reward). points must be non-zero.
//
// This is the module's single most important behavior (its headline
// acceptance criterion): if points exceeds the customer's current balance,
// RedeemPoints refuses the operation and returns an *errs.Error of Kind
// errs.Conflict -- the aggregate is left completely untouched (loyaltyPoints,
// version and updatedAt all unchanged) so a rejected redemption can never
// partially apply. This exactly mirrors
// internal/modules/operations/domain.StockItem.Withdraw's own doc comment
// and behavior for the identical "can't go negative" invariant.
func (c *Customer) RedeemPoints(points LoyaltyPoints) error {
	if points.Points() == 0 {
		return errs.New(errs.Invalid, "invalid_redeem_points",
			"redeem points must not be zero", nil)
	}

	remaining, err := c.loyaltyPoints.Sub(points)
	if err != nil {
		return errs.New(errs.Conflict, "insufficient_loyalty_points",
			"redeem points exceeds customer loyalty point balance", err)
	}

	c.loyaltyPoints = remaining
	c.version++
	c.updatedAt = time.Now().UTC()
	return nil
}
