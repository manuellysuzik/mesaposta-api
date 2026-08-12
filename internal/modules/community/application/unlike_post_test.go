package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
)

func TestUnlikePost_ExistingLike_RemovesIt(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	userID := domain.NewID()
	likeInput := application.LikePostInput{TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID}
	if err := application.LikePost(context.Background(), uow, likeInput); err != nil {
		t.Fatalf("LikePost (setup): %v", err)
	}

	// Act
	err := application.UnlikePost(context.Background(), uow, application.UnlikePostInput{
		TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID,
	})

	// Assert
	if err != nil {
		t.Fatalf("UnlikePost: unexpected error: %v", err)
	}
	if snapshot := uow.snapshot(); len(snapshot.likes) != 0 {
		t.Errorf("likes = %v, want empty after unlike", snapshot.likes)
	}
}

func TestUnlikePost_NeverLiked_IsIdempotent_ReturnsNoError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	err := application.UnlikePost(context.Background(), uow, application.UnlikePostInput{
		TenantID: mustTestTenantID(t), PostID: domain.NewID(), UserID: domain.NewID(),
	})

	// Assert
	if err != nil {
		t.Fatalf("UnlikePost: want nil error (idempotent no-op), got: %v", err)
	}
}

func TestUnlikePost_CalledTwice_SecondCallStillReturnsNoError(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	userID := domain.NewID()
	if err := application.LikePost(context.Background(), uow, application.LikePostInput{
		TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID,
	}); err != nil {
		t.Fatalf("LikePost (setup): %v", err)
	}

	unlikeInput := application.UnlikePostInput{TenantID: mustTestTenantID(t), PostID: seed.ID(), UserID: userID}

	// Act
	err1 := application.UnlikePost(context.Background(), uow, unlikeInput)
	err2 := application.UnlikePost(context.Background(), uow, unlikeInput)

	// Assert
	if err1 != nil {
		t.Fatalf("UnlikePost (1st call): unexpected error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("UnlikePost (2nd call): want nil error (idempotent), got: %v", err2)
	}
}
