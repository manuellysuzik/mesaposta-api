//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// --- CommunityPostRepository -------------------------------------------------

func TestCommunityPostRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCommunityPostRepository(scope)

	authorID := domain.NewID()
	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, authorID, "Bem-vindos ao mural!")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}
	if err := repo.Insert(ctx, &post); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := repo.FindByID(ctx, post.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID() != post.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), post.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.AuthorID() != authorID {
		t.Errorf("found.AuthorID() = %v, want %v", found.AuthorID(), authorID)
	}
	if found.Content() != post.Content() {
		t.Errorf("found.Content() = %q, want %q", found.Content(), post.Content())
	}
	if !found.CreatedAt().Equal(post.CreatedAt()) {
		t.Errorf("found.CreatedAt() = %v, want %v", found.CreatedAt(), post.CreatedAt())
	}
}

func TestCommunityPostRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCommunityPostRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if err == nil {
		t.Fatal("FindByID(unknown id): want an error, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestCommunityPostRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "50000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "50000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewCommunityPostRepository(scopeA)
	post := mustPost(t, ctxA, repoA, tenantA)
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewCommunityPostRepository(scopeB)

	_, err = repoB.FindByID(ctxB, post.ID())
	if err == nil {
		t.Fatal("FindByID: tenant B found tenant A's post, want it hidden by RLS")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

// --- CommentRepository ---------------------------------------------------------

func TestCommentRepository_Insert_Succeeds(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000005")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	postRepo := postgresadapter.NewCommunityPostRepository(scope)
	post := mustPost(t, ctx, postRepo, tenantID)

	commentRepo := postgresadapter.NewCommentRepository(scope)
	comment, err := domain.NewComment(domain.NewID(), tenantID, post.ID(), domain.NewID(), mustCommentText(t, "ótimo!"))
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}
	if err := commentRepo.Insert(ctx, comment); err != nil {
		t.Fatalf("Insert(comment): %v", err)
	}

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM community_comments WHERE id = $1", comment.ID().String()).Scan(&count); err != nil {
		t.Fatalf("counting community_comments: %v", err)
	}
	if count != 1 {
		t.Errorf("community_comments count = %d, want 1", count)
	}
}

func TestCommunityComments_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000006")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	postRepo := postgresadapter.NewCommunityPostRepository(setupScope)
	post := mustPost(t, ctx, postRepo, tenantID)
	commentRepo := postgresadapter.NewCommentRepository(setupScope)
	comment, err := domain.NewComment(domain.NewID(), tenantID, post.ID(), domain.NewID(), mustCommentText(t, "comentario"))
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}
	if err := commentRepo.Insert(ctx, comment); err != nil {
		t.Fatalf("Insert(comment): %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE community_comments SET text = 'tampered' WHERE id = $1", comment.ID().String()); err == nil {
		t.Fatal("UPDATE community_comments: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
	if _, err := scope.Exec(ctx, "DELETE FROM community_comments WHERE id = $1", comment.ID().String()); err == nil {
		t.Fatal("DELETE FROM community_comments: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}

// --- LikeRepository -------------------------------------------------------------

func TestLikeRepository_Insert_FirstLike_ReturnsCreatedTrue(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000007")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	postRepo := postgresadapter.NewCommunityPostRepository(scope)
	post := mustPost(t, ctx, postRepo, tenantID)

	likeRepo := postgresadapter.NewLikeRepository(scope)
	userID := domain.NewID()
	like, err := domain.NewLike(tenantID, post.ID(), userID)
	if err != nil {
		t.Fatalf("NewLike: %v", err)
	}

	created, err := likeRepo.Insert(ctx, like)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !created {
		t.Error("Insert (first like): created = false, want true")
	}
}

// TestLikeRepository_Insert_DuplicateLike_IsIdempotent is this module's
// headline correctness proof: liking the same post twice as the same user
// must affect the database exactly once (the UNIQUE (tenant_id, post_id,
// user_id) constraint plus ON CONFLICT DO NOTHING), never error, and never
// produce two rows.
func TestLikeRepository_Insert_DuplicateLike_IsIdempotent(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000008")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	postRepo := postgresadapter.NewCommunityPostRepository(scope)
	post := mustPost(t, ctx, postRepo, tenantID)

	likeRepo := postgresadapter.NewLikeRepository(scope)
	userID := domain.NewID()
	like, err := domain.NewLike(tenantID, post.ID(), userID)
	if err != nil {
		t.Fatalf("NewLike: %v", err)
	}

	created1, err := likeRepo.Insert(ctx, like)
	if err != nil {
		t.Fatalf("Insert (1st): %v", err)
	}
	created2, err := likeRepo.Insert(ctx, like)
	if err != nil {
		t.Fatalf("Insert (2nd): want nil error (idempotent), got: %v", err)
	}

	if !created1 {
		t.Error("Insert (1st): created = false, want true")
	}
	if created2 {
		t.Error("Insert (2nd, duplicate): created = true, want false")
	}

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM community_likes WHERE post_id = $1 AND user_id = $2",
		post.ID().String(), userID.String()).Scan(&count); err != nil {
		t.Fatalf("counting community_likes: %v", err)
	}
	if count != 1 {
		t.Errorf("community_likes count = %d, want exactly 1 (no duplicate row)", count)
	}
}

func TestLikeRepository_Delete_ExistingLike_RemovesIt(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-000000000009")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	postRepo := postgresadapter.NewCommunityPostRepository(scope)
	post := mustPost(t, ctx, postRepo, tenantID)

	likeRepo := postgresadapter.NewLikeRepository(scope)
	userID := domain.NewID()
	like, err := domain.NewLike(tenantID, post.ID(), userID)
	if err != nil {
		t.Fatalf("NewLike: %v", err)
	}
	if _, err := likeRepo.Insert(ctx, like); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	removed, err := likeRepo.Delete(ctx, post.ID(), userID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !removed {
		t.Error("Delete (existing like): removed = false, want true")
	}
}

func TestLikeRepository_Delete_NeverLiked_IsIdempotent_ReturnsRemovedFalse(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-00000000000a")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	likeRepo := postgresadapter.NewLikeRepository(scope)

	removed, err := likeRepo.Delete(ctx, domain.NewID(), domain.NewID())
	if err != nil {
		t.Fatalf("Delete: want nil error (idempotent no-op), got: %v", err)
	}
	if removed {
		t.Error("Delete (never liked): removed = true, want false")
	}
}

func TestCommunityLikes_Update_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "50000000-0000-0000-0000-00000000000b")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	postRepo := postgresadapter.NewCommunityPostRepository(setupScope)
	post := mustPost(t, ctx, postRepo, tenantID)
	likeRepo := postgresadapter.NewLikeRepository(setupScope)
	userID := domain.NewID()
	like, err := domain.NewLike(tenantID, post.ID(), userID)
	if err != nil {
		t.Fatalf("NewLike: %v", err)
	}
	if _, err := likeRepo.Insert(ctx, like); err != nil {
		t.Fatalf("Insert(like): %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE community_likes SET user_id = $1 WHERE post_id = $2", domain.NewID().String(), post.ID().String()); err == nil {
		t.Fatal("UPDATE community_likes: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
}
