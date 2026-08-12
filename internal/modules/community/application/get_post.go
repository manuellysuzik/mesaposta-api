package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
)

// GetPost is the application-layer use case reading a single CommunityPost
// by id.
//
// It still goes through UnitOfWork.Do rather than reaching directly for a
// standalone repository -- even though this is a plain read with nothing
// to roll back. This is intentional, not an accident: in the real
// Postgres-backed UnitOfWork implementation, Do is where the tenant-scoped,
// row-level-security-enforcing database session gets established (see
// internal/platform/db.Pool.Scope). A getter that bypassed Do to read some
// other way would bypass RLS along with it -- so every read, not just
// every write, is required to flow through the same UnitOfWork/Repositories
// seam. See internal/modules/operations/application.GetStockItem for the
// identical pattern this mirrors.
func GetPost(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.CommunityPost, error) {
	var post *domain.CommunityPost
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Posts().FindByID(ctx, id)
		if err != nil {
			return err
		}
		post = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return post, nil
}
