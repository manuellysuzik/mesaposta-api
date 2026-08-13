package domain

import "context"

// UnitOfWork is the port through which the application layer (../application)
// executes a single atomic operation spanning one or more of the
// repositories below. A concrete (Postgres-backed) implementation MUST run
// fn inside a single database transaction: if fn returns a non-nil error,
// every write attempted through the Repositories handed to fn MUST be
// rolled back in full and never become visible; if fn returns nil, that
// transaction MUST be committed. This mirrors every other module's own
// UnitOfWork in this codebase (e.g.
// internal/modules/crm/domain.UnitOfWork) exactly.
//
// Unlike crm/operations/catalog/community, this module's Repositories does
// NOT expose an outbox.DomainEventPublisher: this phase publishes no
// domain events for ordering (a deliberately deferred scope decision, not
// an oversight -- see this module's final report). Adding one later is a
// backward-compatible extension to this interface, not a breaking change
// to anything that depends on it today.
type UnitOfWork interface {
	// Do invokes fn with a set of Repositories scoped to this single unit
	// of work, committing or rolling back atomically based on fn's return
	// value. Do returns whatever error fn returned (or a wrapping of it)
	// unchanged in errs.Kind, so callers can still classify the failure
	// after it has propagated back through Do.
	Do(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Repositories bundles every repository the ordering module's use cases
// need, all scoped to a single UnitOfWork.Do call so that reads and writes
// performed through them share the same underlying transaction.
type Repositories interface {
	Tables() TableRepository
	Orders() OrderRepository
	TableSessions() TableSessionRepository
}

// TableRepository is the persistence port for the Table aggregate.
//
// UpdateWithVersion is the optimistic-concurrency-control write path: a
// concrete (Postgres-backed) implementation MUST perform the equivalent of
// "UPDATE tables SET ..., version = version + 1 WHERE id = $1 AND
// version = $2", passing expectedVersion as $2, and MUST return an
// *errs.Error of Kind errs.Conflict if that statement affects zero rows --
// mirroring every other module's own UpdateWithVersion contract in this
// codebase (e.g. internal/modules/crm/domain.CustomerRepository).
//
// FindByCode is the read path the PUBLIC cardápio flow uses (see
// ../application.ResolveTableForMenu): it returns whatever row matches
// code, active or not -- the caller, not this repository, is responsible
// for collapsing "no such code" and "code exists but table is inactive"
// into one indistinguishable public response (see that function's own doc
// comment for why that collapsing must happen at the application layer,
// not be silently baked into this query).
type TableRepository interface {
	// FindByID returns the Table identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*Table, error)
	// FindByCode returns the Table identified by code, or a NotFound
	// *errs.Error if none exists -- regardless of whether it is active;
	// see this interface's own doc comment.
	FindByCode(ctx context.Context, code string) (*Table, error)
	// FindAll returns every Table visible to the caller's tenant, ordered
	// ascending by code.
	FindAll(ctx context.Context) ([]*Table, error)
	// Insert persists a brand-new Table.
	Insert(ctx context.Context, table *Table) error
	// UpdateWithVersion persists table's current state, enforcing
	// optimistic concurrency against expectedVersion -- see this
	// interface's doc comment for the exact contract implementations must
	// honor.
	UpdateWithVersion(ctx context.Context, table *Table, expectedVersion int64) error
}

// OrderRepository is the persistence port for the Order aggregate.
// UpdateWithVersion carries the identical optimistic-concurrency contract
// as TableRepository.UpdateWithVersion (see its doc comment).
type OrderRepository interface {
	// FindByID returns the Order identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*Order, error)
	// FindAll returns every Order visible to the caller's tenant, ordered
	// descending by created_at (most recent first).
	FindAll(ctx context.Context) ([]*Order, error)
	// Insert persists a brand-new Order, along with every one of its line
	// items.
	Insert(ctx context.Context, order *Order) error
	// UpdateWithVersion persists order's current state (its status and
	// version/updated_at), enforcing optimistic concurrency against
	// expectedVersion.
	UpdateWithVersion(ctx context.Context, order *Order, expectedVersion int64) error
}

// TableSessionRepository is the persistence port for TableSession. There is
// no update or delete: a TableSession is immutable once minted (it simply
// expires, per TableSession.IsValid) -- mirroring
// internal/platform/auth/refresh's own append-only Record shape, minus
// that package's rotation machinery this module deliberately does not
// need (see table_session.go's own doc comment).
type TableSessionRepository interface {
	// Insert persists a brand-new TableSession.
	Insert(ctx context.Context, session TableSession) error
	// FindByTokenHash returns the TableSession whose TokenHash matches
	// hash, or a NotFound *errs.Error if none exists (or it belongs to a
	// different tenant, per RLS) -- implementations are NOT required to
	// filter out an EXPIRED session's row from this lookup themselves
	// (mirroring internal/platform/auth/refresh.PostgresStore.FindByHash's
	// own choice to do so at the SQL level, purely as an optimization);
	// callers (../application.CreateOrder) always re-check
	// TableSession.IsValid on the result regardless, so either behavior is
	// safe.
	FindByTokenHash(ctx context.Context, hash [32]byte) (*TableSession, error)
}
