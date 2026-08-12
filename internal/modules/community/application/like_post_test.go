package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestLikePost_FirstLike_PersistsLikeAndPublishesEvent(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	userID := domain.NewID()
	input := application.LikePostInput{TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID}

	// Act
	err := application.LikePost(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("LikePost: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	if len(snapshot.likes) != 1 {
		t.Fatalf("likes = %v, want exactly 1", snapshot.likes)
	}
	if len(snapshot.events) != 1 {
		t.Fatalf("events = %v, want exactly 1", snapshot.events)
	}
	if snapshot.events[0].Name != contracts.EventPostLiked {
		t.Errorf("event name = %q, want %q", snapshot.events[0].Name, contracts.EventPostLiked)
	}
}

func TestLikePost_AlreadyLiked_IsIdempotent_PublishesNoSecondEvent(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	userID := domain.NewID()
	input := application.LikePostInput{TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID}

	// Act: like the same post twice as the same user.
	if err := application.LikePost(context.Background(), uow, input); err != nil {
		t.Fatalf("LikePost (1st call): unexpected error: %v", err)
	}
	err := application.LikePost(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("LikePost (2nd call): want nil error (idempotent), got: %v", err)
	}
	snapshot := uow.snapshot()
	if len(snapshot.likes) != 1 {
		t.Errorf("likes = %v, want exactly 1 (no duplicate)", snapshot.likes)
	}
	if len(snapshot.events) != 1 {
		t.Errorf("events = %v, want exactly 1 (no event on the redundant 2nd like)", snapshot.events)
	}
}

func TestLikePost_PostNotFound_ReturnsNotFoundAndPersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.LikePostInput{TenantID: mustTestTenantID(t), PostID: domain.NewID(), UserID: domain.NewID()}

	// Act
	err := application.LikePost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
	if snapshot := uow.snapshot(); len(snapshot.likes) != 0 {
		t.Errorf("likes = %v, want empty", snapshot.likes)
	}
}

func TestLikePost_TwoDifferentUsers_BothPersistedAndBothPublishEvents(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	// Act
	err1 := application.LikePost(context.Background(), uow, application.LikePostInput{
		TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: domain.NewID(),
	})
	err2 := application.LikePost(context.Background(), uow, application.LikePostInput{
		TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: domain.NewID(),
	})

	// Assert
	if err1 != nil || err2 != nil {
		t.Fatalf("LikePost: unexpected errors: %v, %v", err1, err2)
	}
	snapshot := uow.snapshot()
	if len(snapshot.likes) != 2 {
		t.Errorf("likes = %v, want exactly 2 (one per distinct user)", snapshot.likes)
	}
	if len(snapshot.events) != 2 {
		t.Errorf("events = %v, want exactly 2 (one per genuinely new like)", snapshot.events)
	}
}
