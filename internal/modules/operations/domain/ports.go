package domain

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// UnitOfWork is the port through which the application layer (../application)
// executes a single atomic operation spanning one or more of the
// repositories below. A concrete (Postgres-backed) implementation, added in
// a later infrastructure-layer phase, MUST run fn inside a single database
// transaction: if fn returns a non-nil error, every write attempted through
// the Repositories handed to fn MUST be rolled back in full and never
// become visible; if fn returns nil, that transaction MUST be committed.
//
// This atomicity guarantee is the mechanism proving this module's
// acceptance criterion 2 (receiving a purchase order updates the
// PurchaseOrder, every referenced StockItem, the new StockMovements, the
// new Transaction, and publishes every raised domain event as a single,
// all-or-nothing unit). Every use case in ../application MUST perform ALL
// of its persistence -- aggregates and events alike -- through the single
// Repositories value passed into fn, within one Do call; splitting writes
// across multiple Do calls (or reaching around Do to persist directly)
// silently breaks that guarantee.
type UnitOfWork interface {
	// Do invokes fn with a set of Repositories scoped to this single unit
	// of work, committing or rolling back atomically based on fn's return
	// value. Do returns whatever error fn returned (or a wrapping of it)
	// unchanged in errs.Kind, so callers can still classify the failure
	// (e.g. distinguishing errs.Conflict from errs.Invalid) after it has
	// propagated back through Do.
	Do(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Repositories bundles every repository (and the domain event publisher)
// the operations module's use cases need, all scoped to a single
// UnitOfWork.Do call so that reads and writes performed through them share
// the same underlying transaction.
type Repositories interface {
	StockItems() StockItemRepository
	PurchaseOrders() PurchaseOrderRepository
	StockMovements() StockMovementRepository
	Transactions() TransactionRepository
	Events() outbox.DomainEventPublisher
}

// StockItemRepository is the persistence port for the StockItem aggregate.
//
// UpdateWithVersion is the optimistic-concurrency-control write path: a
// concrete (Postgres-backed) implementation MUST perform the equivalent of
// "UPDATE stock_items SET ..., version = version + 1 WHERE id = $1 AND
// version = $2", passing expectedVersion as $2 (the version the caller
// loaded item at, BEFORE the in-memory mutation it is now persisting), and
// MUST return an *errs.Error of Kind errs.Conflict if that statement
// affects zero rows -- whether because the row no longer exists or,
// far more commonly, because a concurrent transaction already advanced its
// version since item was loaded. This interface performs no I/O itself and
// cannot enforce that contract at compile time; honoring it exactly is the
// responsibility of whoever implements it (see ../infrastructure/postgres,
// a later phase).
type StockItemRepository interface {
	// FindByID returns the StockItem identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*StockItem, error)
	// FindManyByIDs returns every StockItem identified by ids, keyed by
	// ID. Implementations are not required to error on a partially missing
	// result -- callers (e.g. domain.ReceivePurchaseOrder, via
	// ../application) are responsible for detecting and reporting any ID
	// absent from the returned map.
	FindManyByIDs(ctx context.Context, ids []ID) (map[ID]*StockItem, error)
	// Insert persists a brand-new StockItem.
	Insert(ctx context.Context, item *StockItem) error
	// UpdateWithVersion persists item's current state, enforcing
	// optimistic concurrency against expectedVersion -- see this
	// interface's doc comment for the exact contract implementations must
	// honor.
	UpdateWithVersion(ctx context.Context, item *StockItem, expectedVersion int64) error
}

// PurchaseOrderRepository is the persistence port for the PurchaseOrder
// aggregate.
//
// FindByIDForUpdate is the write-path lookup: a concrete implementation
// MUST take a row lock (e.g. "SELECT ... FOR UPDATE") so that two
// concurrent receipts of the same order serialize instead of racing.
// FindByID is the read-only counterpart, deliberately added to this
// interface -- beyond the bare minimum the write path alone would need --
// so that the application layer's plain getters (see
// application.GetPurchaseOrder) never have a reason to bypass the
// UnitOfWork/Repositories seam to read a PurchaseOrder some other way:
// every access to a PurchaseOrder, read or write, goes through this same
// port. That uniformity is exactly what lets a future Postgres
// implementation enforce row-level security (RLS) consistently for both
// paths, instead of only for writes.
//
// UpdateWithVersion carries the same optimistic-concurrency contract as
// StockItemRepository.UpdateWithVersion (see its doc comment): "WHERE
// id = $1 AND version = $2", returning an *errs.Error of Kind errs.Conflict
// when that affects zero rows.
type PurchaseOrderRepository interface {
	// FindByID returns the PurchaseOrder identified by id without taking a
	// row lock -- the read-only path, used by plain getters.
	FindByID(ctx context.Context, id ID) (*PurchaseOrder, error)
	// FindByIDForUpdate returns the PurchaseOrder identified by id, taking
	// a row lock so that concurrent writers serialize -- see this
	// interface's doc comment.
	FindByIDForUpdate(ctx context.Context, id ID) (*PurchaseOrder, error)
	// Insert persists a brand-new PurchaseOrder.
	Insert(ctx context.Context, order *PurchaseOrder) error
	// UpdateWithVersion persists order's current state, enforcing
	// optimistic concurrency against expectedVersion -- see this
	// interface's doc comment for the exact contract implementations must
	// honor.
	UpdateWithVersion(ctx context.Context, order *PurchaseOrder, expectedVersion int64) error
}

// StockMovementRepository is the persistence port for the (append-only)
// StockMovement log: there is no update or delete, matching StockMovement's
// own immutable-by-design shape (see domain/stock_movement.go).
type StockMovementRepository interface {
	// Insert persists a brand-new StockMovement.
	Insert(ctx context.Context, movement StockMovement) error
}

// TransactionRepository is the persistence port for the (append-only)
// Transaction log: there is no update or delete, matching Transaction's own
// immutable-by-design shape (see domain/transaction.go).
type TransactionRepository interface {
	// Insert persists a brand-new Transaction.
	Insert(ctx context.Context, transaction Transaction) error
}
