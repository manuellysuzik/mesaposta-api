// Package postgres implements internal/platform/audit's Recorder port
// against a real Postgres database (the audit_log table, see
// internal/platform/db/migrations/0010_audit_log.sql).
//
// This adapter deliberately lives in its own subpackage rather than
// directly inside internal/platform/audit, for exactly the same reason
// internal/platform/outbox/postgres does: the audit port package (Entry,
// Recorder) is on architecture rule R1's domain-platform allowlist,
// meaning **/domain/** packages are allowed to depend on it. If this
// Postgres adapter's own dependencies (pgx, goose, database/sql -- all on
// R1's forbidden list) lived in the audit package itself, importing the
// pure port would transitively drag every one of those into any domain
// package's build graph, silently breaking R1's domain-isolation
// guarantee. Only infrastructure/composition-root code (never domain
// code) should ever import this postgres subpackage.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// Recorder is the audit.Recorder backed by the audit_log table. It
// implements audit.Recorder entirely in terms of db.Scope: every Record
// call opens its own tenant-scoped transaction, so callers that need
// Record to be atomic with the rest of a unit of work should instead call
// WithScope against a *db.Scope they already control (opened once for the
// whole unit of work) -- which is what
// internal/modules/operations/infrastructure/postgres does.
type Recorder struct {
	pool *db.Pool
}

// compile-time check: *Recorder must satisfy audit.Recorder.
var _ audit.Recorder = (*Recorder)(nil)

// NewRecorder builds a Recorder backed by pool.
func NewRecorder(pool *db.Pool) *Recorder {
	return &Recorder{pool: pool}
}

// Record opens a fresh tenant-scoped transaction (via db.Scope, requiring
// ctx to carry an authenticated tenant) and persists entries within it,
// committing on success. An empty entries list is a no-op that never
// touches the database. ctx cancellation is honored both by db.Scope
// itself and by every statement Record issues.
//
// Note the shape this method is NOT: it is not the path a business use
// case should take. A use case that recorded its audit entry through this
// method would be opening a SECOND transaction alongside the one that
// persisted the mutation, reintroducing exactly the "trail that lies"
// failure mode the audit_log exists to rule out. Record is for callers
// that genuinely have nothing else to commit (a standalone administrative
// action, a backfill); everything else uses WithScope.
func (r *Recorder) Record(ctx context.Context, entries ...audit.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	scope, err := r.pool.Scope(ctx)
	if err != nil {
		return fmt.Errorf("audit/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	if err := WithScope(ctx, scope, entries...); err != nil {
		return err
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("audit/postgres: committing audit entries: %w", err)
	}
	return nil
}

// insertEntrySQL inserts a single audit_log row. id is left to the table's
// gen_random_uuid() default. actor_id, before_data and after_data are all
// nullable columns; see nullableActor/nullableJSON below for how an absent
// value becomes a SQL NULL rather than an empty string / JSON null.
const insertEntrySQL = `
INSERT INTO audit_log (tenant_id, actor_id, action, entity_type, entity_id, before_data, after_data, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

// WithScope persists entries within an ALREADY-OPEN scope (typically the
// same *db.Scope a caller used to persist the state change the entries
// describe), so the audit trail's core guarantee holds: either the
// business writes and their audit entries all commit together, or none of
// them do. It does not commit or roll back scope -- that remains the
// caller's responsibility.
//
// Every entry's TenantID is verified against scope.TenantID(): an entry
// whose TenantID does not match the scope it is being recorded through is
// rejected outright rather than silently persisted under a tenant it does
// not belong to. This is the same defense in depth
// internal/platform/outbox/postgres.WithScope applies to domain events,
// and it matters more here, not less: a forged audit entry attributed to
// the wrong tenant is a falsified record, which is precisely the thing an
// audit trail must never contain.
//
// If any entry in the batch fails to insert (a mismatched TenantID, an
// unmarshalable snapshot, a constraint violation, a lost connection, ...),
// NO entry from the batch ends up persisted once the caller's transaction
// is rolled back -- since none of the inserts commit until the caller (or
// Record, for callers that don't manage their own scope) explicitly
// commits the scope.
func WithScope(ctx context.Context, scope *db.Scope, entries ...audit.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	scopeTenant := scope.TenantID()

	for i, entry := range entries {
		if entry.TenantID != scopeTenant {
			return fmt.Errorf("audit/postgres: entry %d (action %q) has tenant_id %s, but the scope is bound to tenant %s: refusing to record",
				i, entry.Action, entry.TenantID, scopeTenant)
		}

		before, err := nullableJSON(entry.Before)
		if err != nil {
			return fmt.Errorf("audit/postgres: marshaling before-snapshot for entry %d (action %q): %w", i, entry.Action, err)
		}
		after, err := nullableJSON(entry.After)
		if err != nil {
			return fmt.Errorf("audit/postgres: marshaling after-snapshot for entry %d (action %q): %w", i, entry.Action, err)
		}

		if _, err := scope.Exec(ctx, insertEntrySQL,
			entry.TenantID.String(), nullableActor(entry.ActorID), entry.Action,
			entry.EntityType, entry.EntityID, before, after, entry.OccurredAt,
		); err != nil {
			return fmt.Errorf("audit/postgres: inserting entry %d (action %q): %w", i, entry.Action, err)
		}
	}

	return nil
}

// nullableActor maps an empty ActorID to a SQL NULL (a nil `any`) rather
// than to the empty string. "No identifiable actor" and "an actor whose
// id happens to be the empty string" must not be storable as the same
// value: NULL says the former unambiguously, and lets a future query
// (`WHERE actor_id IS NULL`) find every system-performed action without
// guessing at a sentinel.
func nullableActor(actorID string) any {
	if actorID == "" {
		return nil
	}
	return actorID
}

// nullableJSON marshals a Before/After snapshot to JSON, mapping a nil
// snapshot to a SQL NULL. Marshaling nil directly would produce the JSON
// literal `null`, which jsonb stores as a present-but-null-valued
// document -- distinct from an absent column, and a distinction the
// "a creation has no before state" case genuinely needs.
func nullableJSON(snapshot any) ([]byte, error) {
	if snapshot == nil {
		return nil, nil
	}
	return json.Marshal(snapshot)
}
