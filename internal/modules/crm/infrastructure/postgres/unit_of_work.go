package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	outboxpg "github.com/manuellysuzik/mesaposta-api/internal/platform/outbox/postgres"
)

// UnitOfWork is the Postgres-backed implementation of domain.UnitOfWork. Do
// opens exactly ONE tenant-scoped db.Scope (a single Postgres transaction)
// per call, hands fn a Repositories bundle wired to that same Scope, and
// commits it if -- and only if -- fn returns nil; any non-nil error rolls
// the whole transaction back, exactly as domain.UnitOfWork's doc comment
// requires. This is the mechanism that makes this module's headline
// acceptance criterion (crediting/redeeming loyalty points updates the
// Customer aggregate and appends its LoyaltyEntry, and publishes the
// raised domain event, as one all-or-nothing unit) true against a REAL
// database, not just against the in-memory fake application/'s own tests
// use (see fakes_test.go). Mirrors
// internal/modules/operations/infrastructure/postgres/unit_of_work.go
// exactly, one repository set narrower.
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
// leaks the underlying pooled connection.
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
		return fmt.Errorf("crm/postgres: committing unit of work: %w", err)
	}
	return nil
}

// repositories is the domain.Repositories implementation Do hands to fn,
// bundling every repository (plus the domain event publisher) scoped to
// the exact same db.Scope -- and therefore the exact same Postgres
// transaction -- for the duration of a single Do call.
type repositories struct {
	scope *db.Scope
}

// compile-time check: *repositories must satisfy domain.Repositories.
var _ domain.Repositories = (*repositories)(nil)

func (r *repositories) Customers() domain.CustomerRepository {
	return NewCustomerRepository(r.scope)
}

func (r *repositories) LoyaltyEntries() domain.LoyaltyEntryRepository {
	return NewLoyaltyEntryRepository(r.scope)
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
type eventPublisher struct {
	scope *db.Scope
}

// compile-time check: eventPublisher must satisfy outbox.DomainEventPublisher.
var _ outbox.DomainEventPublisher = eventPublisher{}

func (p eventPublisher) Publish(ctx context.Context, events ...outbox.DomainEvent) error {
	return outboxpg.WithScope(ctx, p.scope, events...)
}
