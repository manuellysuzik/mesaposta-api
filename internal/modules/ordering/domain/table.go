package domain

import (
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Table is one of a tenant's physical tables, addressed both by an
// authenticated staff member (management CRUD, see ../infrastructure/http)
// and by an unauthenticated diner scanning the QR code printed on/near it
// (via its Code, see ../infrastructure/httppublic and
// ../application.ResolveTableForMenu). It is an aggregate root guarded by
// an optimistic-concurrency Version, incremented on every successful
// mutation (Activate/Deactivate).
type Table struct {
	id        ID
	tenantID  tenant.TenantID
	code      string
	label     string
	isActive  bool
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// initialTableVersion is the Version a freshly constructed Table starts
// at, before any mutation.
const initialTableVersion = 1

// NewTable builds a Table, validating that code and label are both
// non-blank once trimmed. The new table always starts out active (IsActive
// true), at Version 1. Uniqueness of code PER TENANT is enforced by a
// database constraint (see
// internal/platform/db/migrations/0009_ordering.sql), not by this
// constructor -- mirroring
// internal/modules/crm/domain.NewCustomer's identical rationale for
// email/phone uniqueness: NewTable has no way to check other rows without
// I/O, which domain/ must never perform (architecture rule R1).
func NewTable(id ID, tenantID tenant.TenantID, code, label string) (Table, error) {
	code = strings.TrimSpace(code)
	label = strings.TrimSpace(label)

	if code == "" {
		return Table{}, errs.New(errs.Invalid, "invalid_table_code", "table code must not be empty", nil)
	}
	if label == "" {
		return Table{}, errs.New(errs.Invalid, "invalid_table_label", "table label must not be empty", nil)
	}

	now := time.Now().UTC()
	return Table{
		id: id, tenantID: tenantID, code: code, label: label,
		isActive: true, version: initialTableVersion, createdAt: now, updatedAt: now,
	}, nil
}

// ID returns the table's identity.
func (t Table) ID() ID { return t.id }

// TenantID returns the tenant the table belongs to.
func (t Table) TenantID() tenant.TenantID { return t.tenantID }

// Code returns the table's short, QR-code-addressable code.
func (t Table) Code() string { return t.code }

// Label returns the table's human-readable display label (e.g. "Mesa 12").
func (t Table) Label() string { return t.label }

// IsActive returns whether the table currently accepts new orders/menu
// resolution via the public cardápio flow.
func (t Table) IsActive() bool { return t.isActive }

// Version returns the table's optimistic-concurrency version, starting at
// 1 and incrementing by exactly 1 on every successful mutation.
func (t Table) Version() int64 { return t.version }

// CreatedAt returns the timestamp the table was created at.
func (t Table) CreatedAt() time.Time { return t.createdAt }

// UpdatedAt returns the timestamp of the table's last successful mutation
// (or its construction time, if never mutated).
func (t Table) UpdatedAt() time.Time { return t.updatedAt }

// Activate marks the table active, so it can once again be resolved by the
// public cardápio flow. DELIBERATELY IDEMPOTENT: activating an
// already-active table is a no-op -- it does NOT increment Version or
// touch UpdatedAt, and (per ../application's own mutateTable helper) never
// even issues a write to the database -- rather than mirroring
// internal/modules/operations/domain.PurchaseOrder's stricter
// already-received/already-cancelled guard (an *errs.Error of Kind
// errs.Conflict). This is a deliberate, documented technical decision (see
// this module's final report): a "toggle active" management action calling
// Activate twice in a row is far more likely to be a harmless
// double-click/retry than a meaningful conflict worth surfacing as an
// error, unlike an Order's one-way lifecycle transitions.
func (t *Table) Activate() {
	if t.isActive {
		return
	}
	t.isActive = true
	t.version++
	t.updatedAt = time.Now().UTC()
}

// Deactivate marks the table inactive, so the public cardápio flow refuses
// to resolve it (see ../application.ResolveTableForMenu). Symmetrically
// idempotent with Activate -- see that method's own doc comment.
func (t *Table) Deactivate() {
	if !t.isActive {
		return
	}
	t.isActive = false
	t.version++
	t.updatedAt = time.Now().UTC()
}
