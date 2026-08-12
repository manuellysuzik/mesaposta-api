// Package postgres implements internal/platform/outbox's
// DomainEventPublisher port against a real Postgres database (the
// domain_events table, see
// internal/platform/db/migrations/0002_outbox.sql).
//
// This adapter deliberately lives in its own subpackage rather than
// directly inside internal/platform/outbox: the outbox port package
// (DomainEvent, DomainEventPublisher) is on architecture rule R1's
// domain-platform allowlist, meaning **/domain/** packages are allowed to
// depend on it. If this Postgres adapter's own dependencies (pgx, goose,
// database/sql -- all on R1's forbidden list) lived in the outbox package
// itself, importing the pure port would transitively drag every one of
// those into any domain package's build graph, silently breaking R1's
// domain-isolation guarantee the moment Phase 1 introduces a real
// **/domain/** package. Only infrastructure/composition-root code (never
// domain code) should ever import this postgres subpackage.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// Publisher is the transactional-outbox DomainEventPublisher backed by the
// domain_events table. It implements outbox.DomainEventPublisher entirely
// in terms of db.Scope: every Publish call opens its own tenant-scoped
// transaction, so callers that need Publish to be atomic with the rest of
// a unit of work should instead call WithScope against a *db.Scope they
// already control (opened once for the whole unit of work).
type Publisher struct {
	pool *db.Pool
}

// compile-time check: *Publisher must satisfy outbox.DomainEventPublisher.
var _ outbox.DomainEventPublisher = (*Publisher)(nil)

// NewPublisher builds a Publisher backed by pool.
func NewPublisher(pool *db.Pool) *Publisher {
	return &Publisher{pool: pool}
}

// Publish opens a fresh tenant-scoped transaction (via db.Scope, requiring
// ctx to carry an authenticated tenant) and persists events within it,
// committing on success. An empty events list is a no-op that never
// touches the database. ctx cancellation is honored both by db.Scope
// itself and by every statement Publish issues.
func (p *Publisher) Publish(ctx context.Context, events ...outbox.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	scope, err := p.pool.Scope(ctx)
	if err != nil {
		return fmt.Errorf("outbox/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	if err := WithScope(ctx, scope, events...); err != nil {
		return err
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("outbox/postgres: committing published events: %w", err)
	}
	return nil
}

// insertEventSQL inserts a single domain event row. occurred_at and id
// (via the table's default) drive the stable (tenant_id, occurred_at, id)
// replay ordering the domain_events_tenant_order_idx index exists for.
const insertEventSQL = `
INSERT INTO domain_events (tenant_id, aggregate_type, aggregate_id, event_name, payload, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6)
`

// WithScope persists events within an ALREADY-OPEN scope (typically the
// same *db.Scope a caller used to persist the aggregate state the events
// describe), so the classic transactional-outbox guarantee holds: either
// the aggregate's writes and its events all commit together, or none of
// them do. It does not commit or roll back scope -- that remains the
// caller's responsibility.
//
// Every event's TenantID is verified against scope.TenantID(): an event
// whose TenantID does not match the scope it is being published through is
// rejected outright rather than silently persisted under a tenant it does
// not belong to -- a "forged" event claiming another tenant's id must
// never be written, even by a single-statement mistake, let alone by
// something adversarial.
//
// If any event in the batch fails to insert (a mismatched TenantID, a
// constraint violation, a lost connection, ...), NO event from the batch
// ends up persisted once the caller's transaction is rolled back -- since
// none of the inserts commit until the caller (or Publish, for callers
// that don't manage their own scope) explicitly commits the scope.
func WithScope(ctx context.Context, scope *db.Scope, events ...outbox.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}

	scopeTenant := scope.TenantID()

	for i, event := range events {
		if event.TenantID != scopeTenant {
			return fmt.Errorf("outbox/postgres: event %d (name %q) has tenant_id %s, but the scope is bound to tenant %s: refusing to publish",
				i, event.Name, event.TenantID, scopeTenant)
		}

		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("outbox/postgres: marshaling payload for event %d (name %q): %w", i, event.Name, err)
		}

		if _, err := scope.Exec(ctx, insertEventSQL,
			event.TenantID.String(), aggregateTypeOf(event), event.AggregateID, event.Name, payload, event.OccurredAt,
		); err != nil {
			return fmt.Errorf("outbox/postgres: inserting event %d (name %q): %w", i, event.Name, err)
		}
	}

	return nil
}

// aggregateTypeOf derives the aggregate_type column's value. outbox.
// DomainEvent (Bloco B's shared-kernel type) does not itself carry a
// distinct "aggregate type" field, so this uses the dot-namespaced event
// Name's prefix up to its first "." (e.g. "bet.placed" -> "bet") as a
// reasonable, self-consistent default; callers that need a different
// mapping should use WithScope directly against their own INSERT rather
// than rely on this heuristic. Falls back to the full event name when it
// contains no ".".
func aggregateTypeOf(event outbox.DomainEvent) string {
	for i := 0; i < len(event.Name); i++ {
		if event.Name[i] == '.' {
			return event.Name[:i]
		}
	}
	return event.Name
}
