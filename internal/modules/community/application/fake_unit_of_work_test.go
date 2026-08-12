package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
)

// This file exercises fakeUnitOfWork (fakes_test.go) directly, with no
// dependency on any real use case -- proving the fake itself genuinely
// simulates a database transaction's rollback semantics BEFORE any test in
// this package leans on that behavior. Without this guarantee, every
// "nothing was persisted after an error" assertion elsewhere in this
// package would be testing nothing at all.

func mustFakePost(t *testing.T) *domain.CommunityPost {
	t.Helper()
	post, err := domain.NewCommunityPost(domain.NewID(), mustTestTenantID(t), domain.NewID(), "conteudo de teste")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}
	return &post
}

func TestFakeUnitOfWork_Do_CommitsWritesOnSuccess(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	post := mustFakePost(t)

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		return r.Posts().Insert(ctx, post)
	})

	// Assert
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	got, ok := uow.snapshot().posts[post.ID()]
	if !ok {
		t.Fatal("Do: post was not committed to real state after fn returned nil")
	}
	if got.ID() != post.ID() {
		t.Errorf("committed post ID = %v, want %v", got.ID(), post.ID())
	}
}

func TestFakeUnitOfWork_Do_RollsBackOnError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	post := mustFakePost(t)
	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		if insertErr := r.Posts().Insert(ctx, post); insertErr != nil {
			return insertErr
		}
		// A second write, past the first, to prove EVERYTHING attempted
		// within this failed Do call is discarded -- not just the last
		// write before the error.
		comment, commentErr := domain.NewComment(domain.NewID(), mustTestTenantID(t), post.ID(), domain.NewID(), mustFakeCommentText(t))
		if commentErr != nil {
			return commentErr
		}
		if insertErr := r.Comments().Insert(ctx, comment); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.posts) != 0 {
		t.Errorf("Do: posts = %v, want empty after rollback", snapshot.posts)
	}
	if len(snapshot.comments) != 0 {
		t.Errorf("Do: comments = %v, want empty after rollback", snapshot.comments)
	}
}

func TestFakeUnitOfWork_Do_RollbackLeavesPreExistingStateUntouched(t *testing.T) {
	// Arrange: seed the fake with one already-committed post, then run a
	// failing Do call that tries to add a comment to it.
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		comment, commentErr := domain.NewComment(domain.NewID(), mustTestTenantID(t), seed.ID(), domain.NewID(), mustFakeCommentText(t))
		if commentErr != nil {
			return commentErr
		}
		if insertErr := r.Comments().Insert(ctx, comment); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.comments) != 0 {
		t.Errorf("Do: comments = %v, want empty after rollback", snapshot.comments)
	}
	if _, ok := snapshot.posts[seed.ID()]; !ok {
		t.Error("Do: pre-existing post disappeared after a rolled-back Do call")
	}
}

func mustFakeCommentText(t *testing.T) domain.CommentText {
	t.Helper()
	text, err := domain.NewCommentText("comentario de teste")
	if err != nil {
		t.Fatalf("NewCommentText: %v", err)
	}
	return text
}
