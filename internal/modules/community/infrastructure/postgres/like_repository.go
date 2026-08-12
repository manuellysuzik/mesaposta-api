package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// LikeRepository is the Postgres-backed implementation of
// domain.LikeRepository, backed by the community_likes table (see
// internal/platform/db/migrations/0007_community.sql). Both Insert and
// Delete are idempotent, exactly as domain.LikeRepository's own doc comment
// requires:
//
//   - Insert relies on the table's own UNIQUE (tenant_id, post_id, user_id)
//     constraint together with "ON CONFLICT ... DO NOTHING": a like from a
//     user who already likes the post affects zero rows, which Insert
//     reports as created=false rather than a duplicate-key error.
//   - Delete simply removes at most one row matching (post_id, user_id);
//     zero rows affected (no such like existed) is reported as
//     removed=false, never an error.
type LikeRepository struct {
	scope *db.Scope
}

// compile-time check: *LikeRepository must satisfy domain.LikeRepository.
var _ domain.LikeRepository = (*LikeRepository)(nil)

// NewLikeRepository builds a LikeRepository bound to scope. Most callers
// reach a LikeRepository via UnitOfWork.Do/Repositories.Likes() instead;
// this constructor exists so this package's own integration test suite
// (and any other caller that already holds a *db.Scope) can exercise it
// directly.
func NewLikeRepository(scope *db.Scope) *LikeRepository {
	return &LikeRepository{scope: scope}
}

// Insert idempotently records like -- see this type's own doc comment for
// the exact ON CONFLICT DO NOTHING contract this implements.
func (r *LikeRepository) Insert(ctx context.Context, like domain.Like) (bool, error) {
	tag, err := r.scope.Exec(ctx, `
		INSERT INTO community_likes (tenant_id, post_id, user_id, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, post_id, user_id) DO NOTHING
	`, r.scope.TenantID().String(), like.PostID().String(), like.UserID().String(), like.CreatedAt())
	if err != nil {
		return false, fmt.Errorf("community/postgres: inserting like: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Delete idempotently removes the like identified by (postID, userID) --
// see this type's own doc comment for the exact "at most one row, no error
// either way" contract this implements.
func (r *LikeRepository) Delete(ctx context.Context, postID domain.ID, userID domain.ID) (bool, error) {
	tag, err := r.scope.Exec(ctx, `
		DELETE FROM community_likes WHERE post_id = $1 AND user_id = $2
	`, postID.String(), userID.String())
	if err != nil {
		return false, fmt.Errorf("community/postgres: deleting like: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
