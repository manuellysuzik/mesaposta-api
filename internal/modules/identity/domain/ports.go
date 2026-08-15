package domain

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
)

// UnitOfWork is the port through which the application layer (../application)
// executes a single atomic operation spanning one or more of the
// repositories below. A concrete (Postgres-backed) implementation MUST run
// fn inside a single database transaction: if fn returns a non-nil error,
// every write attempted through the Repositories handed to fn MUST be
// rolled back in full and never become visible; if fn returns nil, that
// transaction MUST be committed. Mirrors
// internal/modules/crm/domain.UnitOfWork exactly.
type UnitOfWork interface {
	// Do invokes fn with a set of Repositories scoped to this single unit
	// of work, committing or rolling back atomically based on fn's return
	// value. Do returns whatever error fn returned (or a wrapping of it)
	// unchanged in errs.Kind, so callers can still classify the failure
	// after it has propagated back through Do.
	Do(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Repositories bundles every repository (and the audit recorder) the
// identity module's use cases need, all scoped to a single UnitOfWork.Do
// call so that reads and writes performed through them share the same
// underlying transaction.
//
// Deliberately no Events()/outbox.DomainEventPublisher here, unlike
// internal/modules/crm/domain.Repositories: nothing in this phase raises a
// domain event a user's creation/login/logout needs to publish -- adding
// one is a small, additive change to this interface (and its Postgres/fake
// implementations) whenever a real consumer needs it (YAGNI).
type Repositories interface {
	Users() UserRepository
	// Audit returns the audit.Recorder bound to this unit of work, so an
	// audit entry commits atomically WITH the mutation it describes.
	// Mirrors internal/modules/crm/domain.Repositories.Audit() exactly.
	Audit() audit.Recorder
}

// UserRepository is the persistence port for the User aggregate.
//
// Deliberately no FindByEmail here: that lookup path is NOT tenant-scoped
// (a login request has no tenant established yet) and therefore cannot go
// through UnitOfWork/Repositories at all -- see
// internal/platform/db.Pool.LookupUserCredentialsByEmail, called directly
// by ../application/authenticate_user.go, and that function's own doc
// comment for why this is a deliberate, narrowly-scoped exception rather
// than a gap in this interface.
type UserRepository interface {
	// FindByID returns the User identified by id, or a NotFound *errs.Error
	// if none exists (or none is visible to the caller's tenant, per RLS).
	FindByID(ctx context.Context, id ID) (*User, error)
	// Insert persists a brand-new User. A concrete implementation reports a
	// duplicate email (users.email is UNIQUE globally, see
	// internal/platform/db/migrations/0011_users.sql) as an *errs.Error of
	// Kind errs.Conflict.
	Insert(ctx context.Context, user *User) error
	// ListByTenant returns every User belonging to the caller's tenant (per
	// RLS), ordered by CreatedAt ascending -- backing GET /v1/users (see
	// ../infrastructure/http/user_handlers.go).
	ListByTenant(ctx context.Context) ([]User, error)
}
