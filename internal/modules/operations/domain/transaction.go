package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Transaction is a financial record -- revenue or expense -- posted to a
// tenant's books. It is immutable once created: this phase exposes no
// setter, so correcting a mistaken Transaction means recording a
// compensating one, not editing history in place.
type Transaction struct {
	id              ID
	tenantID        tenant.TenantID
	transactionType TransactionType
	category        TransactionCategory
	description     string
	amount          Money
	date            time.Time
	referenceID     *ID
}

// NewTransaction builds a Transaction, validating that amount is strictly
// positive (a zero-value transaction records nothing and is rejected,
// unlike Money's own constructor which merely forbids negative amounts),
// description is non-empty, transactionType and category are each one of
// their closed set of known values, and date is not the zero time.Time.
// referenceID is optional (e.g. linking an expense Transaction back to the
// PurchaseOrder that generated it) -- there is no foreign key enforcement
// at this phase.
func NewTransaction(
	id ID,
	tenantID tenant.TenantID,
	transactionType TransactionType,
	category TransactionCategory,
	description string,
	amount Money,
	date time.Time,
	referenceID *ID,
) (Transaction, error) {
	if amount.Cents() <= 0 {
		return Transaction{}, errs.New(errs.Invalid, "invalid_transaction_amount",
			"transaction amount must be greater than zero", nil)
	}
	if description == "" {
		return Transaction{}, errs.New(errs.Invalid, "invalid_transaction_description",
			"transaction description must not be empty", nil)
	}
	if !transactionType.Valid() {
		return Transaction{}, errs.New(errs.Invalid, "invalid_transaction_type",
			"transaction type is not one of the known values", nil)
	}
	if !category.Valid() {
		return Transaction{}, errs.New(errs.Invalid, "invalid_transaction_category",
			"transaction category is not one of the known values", nil)
	}
	if date.IsZero() {
		return Transaction{}, errs.New(errs.Invalid, "invalid_transaction_date",
			"transaction date must not be the zero time", nil)
	}

	return Transaction{
		id:              id,
		tenantID:        tenantID,
		transactionType: transactionType,
		category:        category,
		description:     description,
		amount:          amount,
		date:            date,
		referenceID:     referenceID,
	}, nil
}

// ID returns the transaction's identity.
func (t Transaction) ID() ID { return t.id }

// TenantID returns the tenant the transaction belongs to.
func (t Transaction) TenantID() tenant.TenantID { return t.tenantID }

// TransactionType returns whether this is revenue or an expense.
func (t Transaction) TransactionType() TransactionType { return t.transactionType }

// Category returns the transaction's category.
func (t Transaction) Category() TransactionCategory { return t.category }

// Description returns the transaction's human-readable description.
func (t Transaction) Description() string { return t.description }

// Amount returns the (always strictly positive) transaction amount.
func (t Transaction) Amount() Money { return t.amount }

// Date returns the date the transaction was posted for.
func (t Transaction) Date() time.Time { return t.date }

// ReferenceID returns the ID of a related entity (e.g. the PurchaseOrder
// that generated this transaction), or nil if none is set.
func (t Transaction) ReferenceID() *ID { return t.referenceID }
