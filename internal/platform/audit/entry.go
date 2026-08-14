// Package audit defines the shared-kernel "audit log" port: the Entry
// value type and the Recorder interface any layer (including
// internal/modules/**/domain/**, per the R1 platform allowlist in
// architecture/rules.go) may depend on to record an audit-trail entry,
// without coupling to a concrete persistence mechanism.
//
// This package is deliberately a mirror of internal/platform/outbox: both
// solve the exact same shape of problem -- a side effect that MUST be
// persisted atomically, inside the very same transaction as the business
// mutation it describes. An audit entry that commits when the mutation
// rolled back (or the other way round) is worse than no audit trail at
// all, because it is a trail that lies. Keeping the port pure (no pgx, no
// database/sql, no net/http) is what lets a domain package reference
// audit.Recorder without dragging infrastructure into its build graph;
// the concrete Postgres adapter lives in the audit/postgres subpackage.
package audit

import (
	"fmt"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Entry is a single audit-trail record: "actor X did action Y to entity Z
// at time T, taking it from state Before to state After". Every Entry is
// scoped to exactly one tenant.
//
// Entry has no exported constructor other than NewEntry: all fields are
// exported (so callers can read them freely once constructed), but the
// zero value is never valid on its own -- always go through NewEntry,
// which validates eagerly at construction time.
type Entry struct {
	// TenantID is the tenant this entry belongs to. Must not be the zero
	// value.
	TenantID tenant.TenantID
	// ActorID identifies who performed the action. It is intentionally
	// OPTIONAL and intentionally a free-form string rather than a typed
	// id: not every action has a human actor (a scheduled job, a webhook
	// replay, a data migration all legitimately have none), and until an
	// identity module exists the only "who" this system can observe is
	// the authenticated JWT's "sub" claim (see
	// internal/platform/auth.ClaimsFromContext), which is not guaranteed
	// to be a UUID. An empty ActorID means "no identifiable human actor"
	// and is persisted as SQL NULL -- never as the empty string, so that
	// "system did it" and "we forgot to record who did it" stay
	// indistinguishable-by-accident rather than silently conflated with a
	// real actor id. See docs/adr/0006-audit-log.md.
	ActorID string
	// Action is the entry's stable, dot-namespaced identifier for what
	// happened (e.g. "purchase_order.received"). Must not be empty.
	Action string
	// EntityType names the kind of entity acted upon (e.g.
	// "purchase_order"). Must not be empty.
	EntityType string
	// EntityID identifies the specific entity instance acted upon. Must
	// not be empty. It is a string (not a typed id) because the audit log
	// is cross-module: it records entities from every bounded context,
	// none of whose id types this shared-kernel package may know about.
	EntityID string
	// Before is a snapshot of the relevant state BEFORE the action. It is
	// optional (nil is valid): a creation has no "before". It is typed as
	// `any` because this package has no opinion on serialization, which
	// is an infrastructure-layer concern.
	Before any
	// After is a snapshot of the relevant state AFTER the action.
	// Optional (nil is valid): a deletion has no meaningful "after".
	After any
	// OccurredAt is when the action happened (domain time, not
	// necessarily when it was recorded). Must not be the zero time.Time.
	OccurredAt time.Time
}

// NewEntry constructs an Entry, validating every required field at
// construction time -- not deferring validation to whenever the entry is
// later recorded. On any validation failure it returns the zero Entry and
// an *errs.Error with Kind errs.Invalid.
//
// actorID, before and after are deliberately NOT validated: an entry with
// no identifiable actor, no prior state, or no resulting state is a
// legitimate entry (see Entry's field docs), and rejecting those would
// push callers toward inventing placeholder values -- which is strictly
// worse for a trail whose entire value is that it is trustworthy.
func NewEntry(tenantID tenant.TenantID, actorID, action, entityType, entityID string, occurredAt time.Time, before, after any) (Entry, error) {
	if tenantID.IsZero() {
		return Entry{}, errs.New(errs.Invalid, "invalid_audit_entry",
			"audit entry requires a non-zero tenant id", fmt.Errorf("tenant id is zero-value"))
	}
	if action == "" {
		return Entry{}, errs.New(errs.Invalid, "invalid_audit_entry",
			"audit entry requires a non-empty action", fmt.Errorf("action is empty"))
	}
	if entityType == "" {
		return Entry{}, errs.New(errs.Invalid, "invalid_audit_entry",
			"audit entry requires a non-empty entity type", fmt.Errorf("entity type is empty"))
	}
	if entityID == "" {
		return Entry{}, errs.New(errs.Invalid, "invalid_audit_entry",
			"audit entry requires a non-empty entity id", fmt.Errorf("entity id is empty"))
	}
	if occurredAt.IsZero() {
		return Entry{}, errs.New(errs.Invalid, "invalid_audit_entry",
			"audit entry requires a non-zero occurred-at timestamp", fmt.Errorf("occurred at is zero-value"))
	}

	return Entry{
		TenantID:   tenantID,
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Before:     before,
		After:      after,
		OccurredAt: occurredAt,
	}, nil
}
