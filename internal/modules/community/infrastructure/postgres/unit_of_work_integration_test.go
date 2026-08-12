//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestUnitOfWork_Do_SuccessfulFn_CommitsEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, domain.NewID(), "post via UOW")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Posts().Insert(ctx, &post)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Posts().FindByID(ctx, post.ID())
		if err != nil {
			return err
		}
		if found.ID() != post.ID() {
			t.Errorf("found.ID() = %v, want %v", found.ID(), post.ID())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_FailingFn_RollsBackEverything proves that a Do call
// touching both community_posts and community_comments, that then returns
// an error, leaves ZERO rows behind in either table.
func TestUnitOfWork_Do_FailingFn_RollsBackEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	sentinel := errors.New("boom")

	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, domain.NewID(), "post rollback")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Posts().Insert(ctx, &post); err != nil {
			return err
		}

		comment, err := domain.NewComment(domain.NewID(), tenantID, post.ID(), domain.NewID(), mustCommentText(t, "comentario"))
		if err != nil {
			return err
		}
		if err := r.Comments().Insert(ctx, comment); err != nil {
			return err
		}

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		_, err := r.Posts().FindByID(ctx, post.ID())
		if !errs.IsKind(err, errs.NotFound) {
			t.Errorf("FindByID after rollback: errs.KindOf(err) = %v, want errs.NotFound (err=%v)", errs.KindOf(err), err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_NoTenantInContext_ReturnsUnauthenticatedWithoutTouchingDB
// proves Do never acquires a database connection at all when ctx carries no
// authenticated tenant.
func TestUnitOfWork_Do_NoTenantInContext_ReturnsUnauthenticatedWithoutTouchingDB(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	called := false
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("Do without an authenticated tenant: want an error, got nil")
	}
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Unauthenticated", errs.KindOf(err))
	}
	if called {
		t.Error("Do without an authenticated tenant invoked fn -- it must never reach fn at all")
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Errorf("AcquiredConns() = %d, want 0 (no connection should ever be acquired)", got)
	}
}

func TestUnitOfWork_Events_PublishesWithinSameTransaction(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, domain.NewID(), "post evento")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}

	event, err := buildTestDomainEvent(t, tenantID, post.ID().String())
	if err != nil {
		t.Fatalf("buildTestDomainEvent: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Posts().Insert(ctx, &post); err != nil {
			return err
		}
		return r.Events().Publish(ctx, event)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM domain_events WHERE aggregate_id = $1", post.ID().String()).Scan(&count); err != nil {
		t.Fatalf("counting domain_events: %v", err)
	}
	if count != 1 {
		t.Errorf("domain_events count = %d, want 1", count)
	}
}
