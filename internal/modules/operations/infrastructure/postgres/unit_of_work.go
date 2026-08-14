package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	auditpg "github.com/manuellysuzik/mesaposta-api/internal/platform/audit/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	outboxpg "github.com/manuellysuzik/mesaposta-api/internal/platform/outbox/postgres"
)

// UnitOfWork is the Postgres-backed implementation of domain.UnitOfWork. Do
// opens exactly ONE tenant-scoped db.Scope (a single Postgres transaction)
// per call, hands fn a Repositories bundle wired to that same Scope, and
// commits it if -- and only if -- fn returns nil; any non-nil error rolls
// the whole transaction back, exactly as domain.UnitOfWork's doc comment
// requires. This is the mechanism that makes this module's acceptance
// criterion 2 (receiving a purchase order updates every aggregate it
// touches, and publishes every event it raises, as one all-or-nothing
// unit) true against a REAL database, not just against the in-memory fake
// application/'s own tests use (see fakes_test.go).
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
// authenticated tenant -- see db.Pool.Scope's own doc comment; if it does
// not, Scope returns an *errs.Error of Kind errs.Unauthenticated WITHOUT
// acquiring any database connection, so an unauthenticated Do call never
// touches Postgres at all), builds a Repositories bound to that Scope, and
// runs fn against it. fn's returned error (unchanged, so its errs.Kind
// survives) is what Do itself returns; scope.Rollback is deferred
// immediately after a successful Scope call so that ANY early return here
// -- including one triggered by a panic further up the call stack -- never
// leaks the underlying pooled connection (see db.Scope's own doc comment
// for why Rollback is safe, and a no-op, once Commit has already
// succeeded).
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
		return fmt.Errorf("operations/postgres: committing unit of work: %w", err)
	}
	return nil
}

// repositories is the domain.Repositories implementation Do hands to fn,
// bundling every repository (plus the domain event publisher and the
// audit-trail recorder) scoped to the exact same db.Scope -- and therefore
// the exact same Postgres transaction -- for the duration of a single Do
// call.
type repositories struct {
	scope *db.Scope
}

// compile-time check: *repositories must satisfy domain.Repositories.
var _ domain.Repositories = (*repositories)(nil)

func (r *repositories) StockItems() domain.StockItemRepository {
	return NewStockItemRepository(r.scope)
}

func (r *repositories) PurchaseOrders() domain.PurchaseOrderRepository {
	return NewPurchaseOrderRepository(r.scope)
}

func (r *repositories) StockMovements() domain.StockMovementRepository {
	return NewStockMovementRepository(r.scope)
}

func (r *repositories) Transactions() domain.TransactionRepository {
	return NewTransactionRepository(r.scope)
}

func (r *repositories) Events() outbox.DomainEventPublisher {
	return eventPublisher{scope: r.scope}
}

// eventPublisher adapts outboxpg.WithScope (a free function operating on an
// already-open *db.Scope) to the outbox.DomainEventPublisher interface,
// scoped to the SAME transaction every other repository handed out by this
// Do call uses -- the classic transactional-outbox guarantee: either the
// aggregate writes and their events all commit together, or (per Do's own
// deferred Rollback) none of them do.
//
// Every event's TenantID is checked by outboxpg.WithScope against
// scope.TenantID() (the tenant Do's own db.Pool.Scope call derived from
// ctx): a domain event whose TenantID does not match -- e.g. a caller bug
// that built it from the wrong tenant -- is rejected outright rather than
// silently persisted under a tenant it does not belong to. Every event
// application/'s use cases construct is built from an aggregate's own
// TenantID(), which always traces back to the same authenticated tenant
// ctx carries into Do, so this check is never expected to fail in
// practice -- but it is exactly the kind of defense-in-depth that turns "a
// bug that would have forged cross-tenant data" into "a loud, immediate
// error" instead.
type eventPublisher struct {
	scope *db.Scope
}

// compile-time check: eventPublisher must satisfy outbox.DomainEventPublisher.
var _ outbox.DomainEventPublisher = eventPublisher{}

func (p eventPublisher) Publish(ctx context.Context, events ...outbox.DomainEvent) error {
	return outboxpg.WithScope(ctx, p.scope, events...)
}

func (r *repositories) Audit() audit.Recorder {
	return auditRecorder{scope: r.scope}
}

// auditRecorder adapts auditpg.WithScope (a free function operating on an
// already-open *db.Scope) to the audit.Recorder interface, scoped to the
// SAME transaction every other repository handed out by this Do call uses.
// This is deliberately the exact shape eventPublisher above has, for the
// exact same reason: the audit entry describing a mutation must commit
// with that mutation or not at all. Deliberately NOT used here:
// auditpg.Recorder (the pool-backed adapter), which opens a transaction of
// its own -- reaching for it inside a use case would silently reintroduce
// the two-transaction split this whole seam exists to prevent.
//
// Every entry's TenantID is checked by auditpg.WithScope against
// scope.TenantID() (the tenant Do's own db.Pool.Scope call derived from
// ctx): an entry whose TenantID does not match -- e.g. a caller bug that
// built it from the wrong tenant -- is rejected outright rather than
// silently recorded under a tenant it does not belong to. Every entry
// application/'s use cases construct is built from an aggregate's own
// TenantID(), which always traces back to the same authenticated tenant
// ctx carries into Do, so this check is never expected to fire in
// practice -- but a falsified audit record is exactly the failure this
// kind of defense in depth exists to turn into a loud, immediate error.
type auditRecorder struct {
	scope *db.Scope
}

// compile-time check: auditRecorder must satisfy audit.Recorder.
var _ audit.Recorder = auditRecorder{}

func (r auditRecorder) Record(ctx context.Context, entries ...audit.Entry) error {
	return auditpg.WithScope(ctx, r.scope, entries...)
}
