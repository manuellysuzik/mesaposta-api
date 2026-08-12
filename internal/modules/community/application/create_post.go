package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreatePostInput carries everything CreatePost needs to build a
// brand-new CommunityPost.
type CreatePostInput struct {
	TenantID tenant.TenantID
	AuthorID domain.ID
	Content  string
}

// CreatePost is the application-layer use case that creates a new
// CommunityPost. It builds the aggregate via domain.NewCommunityPost --
// which performs all field validation -- and, only if that succeeds,
// persists it via Posts().Insert within a single UnitOfWork.Do call.
func CreatePost(ctx context.Context, uow domain.UnitOfWork, input CreatePostInput) (domain.CommunityPost, error) {
	post, err := domain.NewCommunityPost(domain.NewID(), input.TenantID, input.AuthorID, input.Content)
	if err != nil {
		return domain.CommunityPost{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Posts().Insert(ctx, &post)
	})
	if err != nil {
		return domain.CommunityPost{}, err
	}

	return post, nil
}
