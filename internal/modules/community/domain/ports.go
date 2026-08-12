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
// internal/modules/operations/domain.UnitOfWork exactly -- see that type's
// own doc comment for the full rationale.
type UnitOfWork interface {
	// Do invokes fn with a set of Repositories scoped to this single unit
	// of work, committing or rolling back atomically based on fn's return
	// value. Do returns whatever error fn returned (or a wrapping of it)
	// unchanged in errs.Kind, so callers can still classify the failure
	// (e.g. distinguishing errs.NotFound from errs.Invalid) after it has
	// propagated back through Do.
	Do(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Repositories bundles every repository (and the domain event publisher)
// the community module's use cases need, all scoped to a single
// UnitOfWork.Do call so that reads and writes performed through them share
// the same underlying transaction.
type Repositories interface {
	Posts() CommunityPostRepository
	Comments() CommentRepository
	Likes() LikeRepository
	Events() outbox.DomainEventPublisher
}

// CommunityPostRepository is the persistence port for the CommunityPost
// aggregate. There is no UpdateWithVersion here, unlike
// internal/modules/operations' aggregates: CommunityPost is immutable once
// created (see community_post.go's own doc comment), so there is nothing
// to update.
type CommunityPostRepository interface {
	// FindByID returns the CommunityPost identified by id, or a NotFound
	// *errs.Error if none exists (or none is visible to the caller's
	// tenant -- see internal/modules/README.md, R6, on why a cross-tenant
	// lookup and a genuinely missing one must be indistinguishable to the
	// caller).
	FindByID(ctx context.Context, id ID) (*CommunityPost, error)
	// Insert persists a brand-new CommunityPost.
	Insert(ctx context.Context, post *CommunityPost) error
}

// CommentRepository is the persistence port for the (append-only) Comment
// log: there is no update or delete, matching Comment's own immutable-by-
// design shape (see comment.go) -- the same append-only pattern
// internal/modules/operations/domain.StockMovementRepository already
// establishes for StockMovement.
type CommentRepository interface {
	// Insert persists a brand-new Comment.
	Insert(ctx context.Context, comment Comment) error
}

// LikeRepository is the persistence port for the Like aggregate. Both of
// its methods are idempotent BY DESIGN -- this is what makes
// PUT/DELETE /v1/posts/{id}/curtidas/me idempotent at the HTTP layer: a
// concrete (Postgres-backed) implementation is expected to enforce this via
// the UNIQUE (tenant_id, post_id, user_id) constraint on the underlying
// table together with "INSERT ... ON CONFLICT DO NOTHING" (see
// internal/platform/db/migrations/0007_community.sql and like.go's own doc
// comment on why Like is a separate aggregate from CommunityPost).
type LikeRepository interface {
	// Insert idempotently records like. If a like from the same user on the
	// same post already exists, Insert is a no-op and returns
	// created=false; otherwise it persists the new row and returns
	// created=true. Callers (see ../application.LikePost) use created to
	// decide whether to publish a PostLiked domain event -- exactly once
	// per genuinely new like, never on a redundant repeat call.
	Insert(ctx context.Context, like Like) (created bool, err error)
	// Delete idempotently removes the like identified by (postID, userID).
	// If no such like exists, Delete is a no-op and returns removed=false;
	// otherwise it deletes the row and returns removed=true.
	Delete(ctx context.Context, postID ID, userID ID) (removed bool, err error)
}
