package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CommentOnPostInput carries everything CommentOnPost needs to add a new
// comment to an existing CommunityPost.
type CommentOnPostInput struct {
	TenantID tenant.TenantID
	PostID   domain.ID
	AuthorID domain.ID
	Text     string
}

// CommentOnPost is the application-layer use case that adds a comment to
// an existing CommunityPost. It confirms the post exists (and is visible
// to the caller's tenant, per RLS) BEFORE persisting anything -- a comment
// on a nonexistent or cross-tenant post is reported as errs.NotFound, not
// silently accepted -- then validates Text via domain.NewCommentText,
// builds the Comment via domain.NewComment, persists it, and publishes a
// contracts.EventPostCommented event, all atomically within a single
// UnitOfWork.Do call.
func CommentOnPost(ctx context.Context, uow domain.UnitOfWork, input CommentOnPostInput) (domain.Comment, error) {
	text, err := domain.NewCommentText(input.Text)
	if err != nil {
		return domain.Comment{}, err
	}

	var comment domain.Comment
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if _, err := r.Posts().FindByID(ctx, input.PostID); err != nil {
			return err
		}

		built, err := domain.NewComment(domain.NewID(), input.TenantID, input.PostID, input.AuthorID, text)
		if err != nil {
			return err
		}

		if err := r.Comments().Insert(ctx, built); err != nil {
			return err
		}

		event, err := postCommentedEvent(built)
		if err != nil {
			return err
		}
		if err := r.Events().Publish(ctx, event); err != nil {
			return err
		}

		comment = built
		return nil
	})
	if err != nil {
		return domain.Comment{}, err
	}

	return comment, nil
}

// postCommentedEvent builds the EventPostCommented domain event for a
// newly persisted comment.
func postCommentedEvent(comment domain.Comment) (outbox.DomainEvent, error) {
	return outbox.NewDomainEvent(
		comment.TenantID(),
		contracts.EventPostCommented,
		comment.PostID().String(),
		comment.CreatedAt(),
		contracts.PostCommentedPayload{
			PostID:     comment.PostID().String(),
			CommentID:  comment.ID().String(),
			AuthorID:   comment.AuthorID().String(),
			OccurredAt: comment.CreatedAt(),
		},
	)
}
