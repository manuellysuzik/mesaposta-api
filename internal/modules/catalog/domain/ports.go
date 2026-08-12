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
// transaction MUST be committed. This mirrors
// internal/modules/operations/domain.UnitOfWork exactly -- see that
// interface's doc comment for the full rationale.
type UnitOfWork interface {
	// Do invokes fn with a set of Repositories scoped to this single unit
	// of work, committing or rolling back atomically based on fn's return
	// value. Do returns whatever error fn returned (or a wrapping of it)
	// unchanged in errs.Kind, so callers can still classify the failure
	// after it has propagated back through Do.
	Do(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Repositories bundles every repository (and the domain event publisher)
// the catalog module's use cases need, all scoped to a single
// UnitOfWork.Do call so that reads and writes performed through them share
// the same underlying transaction.
type Repositories interface {
	Products() ProductRepository
	Recipes() RecipeRepository
	Events() outbox.DomainEventPublisher
}

// ProductRepository is the persistence port for the Product aggregate.
//
// UpdateWithVersion is the optimistic-concurrency-control write path: a
// concrete (Postgres-backed) implementation MUST perform the equivalent of
// "UPDATE products SET ..., version = version + 1 WHERE id = $1 AND
// version = $2", passing expectedVersion as $2, and MUST return an
// *errs.Error of Kind errs.Conflict if that statement affects zero rows --
// mirroring internal/modules/operations/domain.StockItemRepository's own
// documented contract exactly.
type ProductRepository interface {
	// FindByID returns the Product identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*Product, error)
	// FindActive returns every active Product visible to the caller's
	// tenant, ordered ascending by name.
	FindActive(ctx context.Context) ([]*Product, error)
	// Insert persists a brand-new Product.
	Insert(ctx context.Context, product *Product) error
	// UpdateWithVersion persists product's current state, enforcing
	// optimistic concurrency against expectedVersion -- see this
	// interface's doc comment for the exact contract implementations must
	// honor.
	UpdateWithVersion(ctx context.Context, product *Product, expectedVersion int64) error
}

// RecipeRepository is the persistence port for the Recipe aggregate. This
// phase's use cases never mutate a persisted Recipe (see recipe.go's own
// doc comment on why Version never advances), so -- unlike
// ProductRepository -- there is no UpdateWithVersion method here; adding
// one before a use case actually needs it would violate this project's
// YAGNI guidance (internal/modules/README.md, common/coding-style.md).
type RecipeRepository interface {
	// FindByID returns the Recipe identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant).
	FindByID(ctx context.Context, id ID) (*Recipe, error)
	// Insert persists a brand-new Recipe.
	Insert(ctx context.Context, recipe *Recipe) error
}
