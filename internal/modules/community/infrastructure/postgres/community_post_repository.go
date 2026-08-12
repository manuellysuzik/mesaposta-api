package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CommunityPostRepository is the Postgres-backed implementation of
// domain.CommunityPostRepository, backed by the community_posts table (see
// internal/platform/db/migrations/0007_community.sql). Every method runs
// against an already-open db.Scope -- there is deliberately no
// "tenant_id = $n" predicate in any SELECT below, since Postgres row-level
// security (the tenant_isolation policy on community_posts) already
// restricts every statement to the scope's own tenant. Every rehydrated
// CommunityPost's TenantID is set from scope.TenantID() -- never re-parsed
// from the row's own tenant_id column (which architecture rule R6 would
// forbid this package from doing via tenant.Parse anyway) -- since RLS
// already guarantees that column can only ever equal the scope's tenant.
type CommunityPostRepository struct {
	scope *db.Scope
}

// compile-time check: *CommunityPostRepository must satisfy domain.CommunityPostRepository.
var _ domain.CommunityPostRepository = (*CommunityPostRepository)(nil)

// NewCommunityPostRepository builds a CommunityPostRepository bound to
// scope. Most callers reach a CommunityPostRepository via
// UnitOfWork.Do/Repositories.Posts() instead; this constructor exists so
// this package's own integration test suite (and any other caller that
// already holds a *db.Scope) can exercise it directly.
func NewCommunityPostRepository(scope *db.Scope) *CommunityPostRepository {
	return &CommunityPostRepository{scope: scope}
}

// postNotFoundErr builds the *errs.Error a missing-row lookup returns.
func postNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "post_not_found", "post not found", cause)
}

const selectCommunityPostColumns = `id, author_id, content, created_at`

// FindByID returns the CommunityPost identified by id, or a NotFound
// *errs.Error if none exists (or none is visible to the scope's tenant,
// per RLS).
func (r *CommunityPostRepository) FindByID(ctx context.Context, id domain.ID) (*domain.CommunityPost, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectCommunityPostColumns+" FROM community_posts WHERE id = $1", id.String())

	post, err := scanCommunityPostRow(row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, postNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("community/postgres: finding post by id: %w", err)
	}
	return post, nil
}

// Insert persists a brand-new CommunityPost.
func (r *CommunityPostRepository) Insert(ctx context.Context, post *domain.CommunityPost) error {
	_, err := r.scope.Exec(ctx, `
		INSERT INTO community_posts (id, tenant_id, author_id, content, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`,
		post.ID().String(), r.scope.TenantID().String(), post.AuthorID().String(), post.Content(), post.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("community/postgres: inserting post: %w", err)
	}
	return nil
}

// communityPostRow is satisfied by pgx.Row, the shape FindByID's single-row
// query returns.
type communityPostRow interface {
	Scan(dest ...any) error
}

// scanCommunityPostRow scans a single community_posts row (in the exact
// column order selectCommunityPostColumns selects) and rehydrates it into a
// *domain.CommunityPost via domain.NewCommunityPost plus the one field that
// constructor cannot know about (CreatedAt) -- see hydrate.go's doc comment
// for why that requires setUnexportedField instead of a public domain
// constructor.
func scanCommunityPostRow(row communityPostRow, tenantID tenant.TenantID) (*domain.CommunityPost, error) {
	var (
		idStr     string
		authorStr string
		content   string
		createdAt time.Time
	)

	if err := row.Scan(&idStr, &authorStr, &content, &createdAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("community/postgres: parsing post id: %w", err)
	}
	authorID, err := domain.ParseID(authorStr)
	if err != nil {
		return nil, fmt.Errorf("community/postgres: parsing post author id: %w", err)
	}

	post, err := domain.NewCommunityPost(id, tenantID, authorID, content)
	if err != nil {
		return nil, fmt.Errorf("community/postgres: rehydrating post: %w", err)
	}

	setUnexportedField(&post, "createdAt", createdAt.UTC())

	return &post, nil
}
