package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// LikePostInput carries everything LikePost needs to record that a user
// likes a CommunityPost.
type LikePostInput struct {
	TenantID tenant.TenantID
	PostID   domain.ID
	UserID   domain.ID
}

// LikePost is the application-layer use case behind the idempotent
// PUT /v1/posts/{id}/curtidas/me: it confirms the post exists (errs.NotFound
// otherwise), then records the like via LikeRepository.Insert, which is
// itself idempotent (see domain.LikeRepository's own doc comment) --
// liking a post you already like is a genuine no-op, never an error.
//
// A contracts.EventPostLiked domain event is published only when Insert
// reports created=true, i.e. only the first time a given user likes a
// given post -- a redundant, already-liked call publishes nothing, so
// downstream consumers of this event never see a duplicate "liked"
// notification for the same (post, user) pair.
func LikePost(ctx context.Context, uow domain.UnitOfWork, input LikePostInput) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if _, err := r.Posts().FindByID(ctx, input.PostID); err != nil {
			return err
		}

		like, err := domain.NewLike(input.TenantID, input.PostID, input.UserID)
		if err != nil {
			return err
		}

		created, err := r.Likes().Insert(ctx, like)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}

		event, err := postLikedEvent(like)
		if err != nil {
			return err
		}
		return r.Events().Publish(ctx, event)
	})
}

// postLikedEvent builds the EventPostLiked domain event for a newly
// created like.
func postLikedEvent(like domain.Like) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		like.TenantID(),
		contracts.EventPostLiked,
		like.PostID().String(),
		like.CreatedAt(),
		contracts.PostLikedPayload{
			PostID:     like.PostID().String(),
			UserID:     like.UserID().String(),
			OccurredAt: like.CreatedAt(),
		},
	)
}
