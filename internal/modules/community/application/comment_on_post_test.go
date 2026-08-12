package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCommentOnPost_Valid_PersistsCommentAndPublishesEvent(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	authorID := domain.NewID()
	input := application.CommentOnPostInput{
		TenantID: mustTestTenantID(t),
		PostID:   seed.ID(),
		AuthorID: authorID,
		Text:     "excelente iniciativa!",
	}

	// Act
	comment, err := application.CommentOnPost(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CommentOnPost: unexpected error: %v", err)
	}
	if comment.PostID() != seed.ID() {
		t.Errorf("comment.PostID() = %v, want %v", comment.PostID(), seed.ID())
	}
	if comment.AuthorID() != authorID {
		t.Errorf("comment.AuthorID() = %v, want %v", comment.AuthorID(), authorID)
	}

	snapshot := uow.snapshot()
	if len(snapshot.comments) != 1 {
		t.Fatalf("comments = %v, want exactly 1", snapshot.comments)
	}
	if snapshot.comments[0].Text().String() != "excelente iniciativa!" {
		t.Errorf("persisted comment text = %q, want %q", snapshot.comments[0].Text().String(), "excelente iniciativa!")
	}
	if len(snapshot.events) != 1 {
		t.Fatalf("events = %v, want exactly 1", snapshot.events)
	}
	if snapshot.events[0].Name != contracts.EventPostCommented {
		t.Errorf("event name = %q, want %q", snapshot.events[0].Name, contracts.EventPostCommented)
	}
}

func TestCommentOnPost_PostNotFound_ReturnsNotFoundAndPersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CommentOnPostInput{
		TenantID: mustTestTenantID(t),
		PostID:   domain.NewID(),
		AuthorID: domain.NewID(),
		Text:     "comentario valido",
	}

	// Act
	_, err := application.CommentOnPost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
	if snapshot := uow.snapshot(); len(snapshot.comments) != 0 {
		t.Errorf("comments = %v, want empty", snapshot.comments)
	}
}

func TestCommentOnPost_EmptyText_ReturnsInvalid_NeverCallsUnitOfWork(t *testing.T) {
	// Arrange: no post seeded -- if CommentOnPost incorrectly called
	// uow.Do before validating Text, this would surface as a NotFound
	// instead of the expected Invalid, since FindByID would run first.
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CommentOnPostInput{
		TenantID: mustTestTenantID(t),
		PostID:   domain.NewID(),
		AuthorID: domain.NewID(),
		Text:     "   ",
	}

	// Act
	_, err := application.CommentOnPost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}
