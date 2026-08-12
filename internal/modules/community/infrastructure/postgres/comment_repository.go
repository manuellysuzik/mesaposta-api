package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// CommentRepository is the Postgres-backed implementation of
// domain.CommentRepository, backed by the (append-only) community_comments
// table (see internal/platform/db/migrations/0007_community.sql). It
// exposes only Insert, matching domain.CommentRepository's own append-only
// shape -- there is no update/delete method to implement, and the
// underlying table's own GRANT (SELECT, INSERT only, no UPDATE/DELETE)
// makes that immutability true at the database layer too, mirroring
// internal/modules/operations/infrastructure/postgres/stock_movement_repository.go
// and internal/modules/crm/infrastructure/postgres/loyalty_entry_repository.go.
type CommentRepository struct {
	scope *db.Scope
}

// compile-time check: *CommentRepository must satisfy domain.CommentRepository.
var _ domain.CommentRepository = (*CommentRepository)(nil)

// NewCommentRepository builds a CommentRepository bound to scope. Most
// callers reach a CommentRepository via
// UnitOfWork.Do/Repositories.Comments() instead; this constructor exists so
// this package's own integration test suite (and any other caller that
// already holds a *db.Scope) can exercise it directly.
func NewCommentRepository(scope *db.Scope) *CommentRepository {
	return &CommentRepository{scope: scope}
}

// Insert persists a brand-new Comment.
func (r *CommentRepository) Insert(ctx context.Context, comment domain.Comment) error {
	_, err := r.scope.Exec(ctx, `
		INSERT INTO community_comments (id, tenant_id, post_id, author_id, text, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
		comment.ID().String(), r.scope.TenantID().String(), comment.PostID().String(),
		comment.AuthorID().String(), comment.Text().String(), comment.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("community/postgres: inserting comment: %w", err)
	}
	return nil
}
