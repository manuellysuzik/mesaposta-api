package domain

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// UnitOfWork is the port through which the application layer (../application)
// executes a single atomic operation spanning one or more of the
// repositories below. A concrete (Postgres-backed) implementation MUST run
// fn inside a single database transaction: if fn returns a non-nil error,
// every write attempted through the Repositories handed to fn MUST be
// rolled back in full and never become visible; if fn returns nil, that
// transaction MUST be committed.
//
// This atomicity guarantee is the mechanism proving this module's headline
// acceptance criterion at the persistence layer: crediting or redeeming
// loyalty points updates the Customer's balance/version AND appends the
// matching LoyaltyEntry as a single, all-or-nothing unit -- see
// customer.go's own doc comment for why that atomicity is load-bearing for
// the "can't redeem more than the balance" invariant under concurrency, not
// just a tidiness preference. Every use case in ../application MUST perform
// ALL of its persistence -- the aggregate and its ledger entry alike --
// through the single Repositories value passed into fn, within one Do call.
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
// the crm module's use cases need, all scoped to a single UnitOfWork.Do
// call so that reads and writes performed through them share the same
// underlying transaction.
type Repositories interface {
	Customers() CustomerRepository
	LoyaltyEntries() LoyaltyEntryRepository
	Events() outbox.DomainEventPublisher
}

// CustomerRepository is the persistence port for the Customer aggregate
// (which, per this module's aggregate boundary decision, includes its
// LoyaltyEntry ledger's invariant, even though ledger rows themselves are
// persisted through LoyaltyEntryRepository below).
//
// UpdateWithVersion is the optimistic-concurrency-control write path: a
// concrete (Postgres-backed) implementation MUST perform the equivalent of
// "UPDATE customers SET ..., version = version + 1 WHERE id = $1 AND
// version = $2", passing expectedVersion as $2 (the version the caller
// loaded the customer at, BEFORE the in-memory mutation it is now
// persisting), and MUST return an *errs.Error of Kind errs.Conflict if that
// statement affects zero rows -- whether because the row no longer exists
// or, far more commonly, because a concurrent transaction already advanced
// its version since the customer was loaded. This interface performs no
// I/O itself and cannot enforce that contract at compile time; honoring it
// exactly is the responsibility of whoever implements it (see
// ../infrastructure/postgres).
type CustomerRepository interface {
	// FindByID returns the Customer identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*Customer, error)
	// Insert persists a brand-new Customer.
	Insert(ctx context.Context, customer *Customer) error
	// UpdateWithVersion persists customer's current state, enforcing
	// optimistic concurrency against expectedVersion -- see this
	// interface's doc comment for the exact contract implementations must
	// honor.
	UpdateWithVersion(ctx context.Context, customer *Customer, expectedVersion int64) error
}

// LoyaltyEntryRepository is the persistence port for the (append-only)
// LoyaltyEntry ledger: there is no update or delete, matching
// LoyaltyEntry's own immutable-by-design shape (see domain/loyalty_entry.go).
type LoyaltyEntryRepository interface {
	// Insert persists a brand-new LoyaltyEntry.
	Insert(ctx context.Context, entry LoyaltyEntry) error
}
