package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	auditpg "github.com/manuellysuzik/mesaposta-api/internal/platform/audit/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// UnitOfWork is the Postgres-backed implementation of domain.UnitOfWork. Do
// opens exactly ONE tenant-scoped db.Scope (a single Postgres transaction)
// per call, hands fn a Repositories bundle wired to that same Scope, and
// commits it if -- and only if -- fn returns nil; any non-nil error rolls
// the whole transaction back, exactly as domain.UnitOfWork's doc comment
// requires. Mirrors every other module's own UnitOfWork in this codebase
// (e.g. internal/modules/crm/infrastructure/postgres/unit_of_work.go)
// exactly, minus the outbox.DomainEventPublisher wiring those modules
// include -- this module publishes no domain events in this phase (see
// domain/ports.go's own doc comment).
type UnitOfWork struct {
	pool *db.Pool
}

// compile-time check: *UnitOfWork must satisfy domain.UnitOfWork.
var _ domain.UnitOfWork = (*UnitOfWork)(nil)

// NewUnitOfWork builds a UnitOfWork backed by pool.
func NewUnitOfWork(pool *db.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

// Do opens a single db.Pool.Scope (requiring ctx to already carry an
// authenticated tenant -- see db.Pool.Scope's own doc comment), builds a
// Repositories bound to that Scope, and runs fn against it.
func (u *UnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	scope, err := u.pool.Scope(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = scope.Rollback(ctx) }()

	repos := &repositories{scope: scope}

	if err := fn(ctx, repos); err != nil {
		return err
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("ordering/postgres: committing unit of work: %w", err)
	}
	return nil
}

// repositories is the domain.Repositories implementation Do hands to fn,
// bundling every repository scoped to the exact same db.Scope -- and
// therefore the exact same Postgres transaction -- for the duration of a
// single Do call.
type repositories struct {
	scope *db.Scope
}

// compile-time check: *repositories must satisfy domain.Repositories.
var _ domain.Repositories = (*repositories)(nil)

func (r *repositories) Tables() domain.TableRepository {
	return NewTableRepository(r.scope)
}

func (r *repositories) Orders() domain.OrderRepository {
	return NewOrderRepository(r.scope)
}

func (r *repositories) TableSessions() domain.TableSessionRepository {
	return NewTableSessionRepository(r.scope)
}

func (r *repositories) Audit() audit.Recorder {
	return auditRecorder{scope: r.scope}
}

// auditRecorder adapts auditpg.WithScope to the audit.Recorder interface,
// scoped to the SAME transaction every other repository handed out by this
// Do call uses. Mirrors every other module's own auditRecorder exactly.
type auditRecorder struct {
	scope *db.Scope
}

// compile-time check: auditRecorder must satisfy audit.Recorder.
var _ audit.Recorder = auditRecorder{}

func (r auditRecorder) Record(ctx context.Context, entries ...audit.Entry) error {
	return auditpg.WithScope(ctx, r.scope, entries...)
}
