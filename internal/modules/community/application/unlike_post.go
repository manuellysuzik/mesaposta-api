package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// UnlikePostInput carries everything UnlikePost needs to remove a user's
// like from a CommunityPost.
type UnlikePostInput struct {
	TenantID tenant.TenantID
	PostID   domain.ID
	UserID   domain.ID
}

// UnlikePost is the application-layer use case behind the idempotent
// DELETE /v1/posts/{id}/curtidas/me: it removes the like via
// LikeRepository.Delete, which is itself idempotent (see
// domain.LikeRepository's own doc comment) -- unliking a post you never
// liked (or that no longer exists) is a genuine no-op, never an error.
//
// Unlike LikePost, UnlikePost deliberately does NOT confirm the post
// exists first: a DELETE-shaped operation removing "at most one row that
// may or may not be there" is idempotent regardless of whether its parent
// resource is still around, and skipping that extra lookup avoids a wasted
// round trip on the overwhelmingly common case (the like simply exists, or
// simply doesn't).
func UnlikePost(ctx context.Context, uow domain.UnitOfWork, input UnlikePostInput) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		_, err := r.Likes().Delete(ctx, input.PostID, input.UserID)
		return err
	})
}
