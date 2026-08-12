package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreatePost_Valid_PersistsPost(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreatePostInput{
		TenantID: mustTestTenantID(t),
		AuthorID: domain.NewID(),
		Content:  "Bem-vindos ao mural da equipe!",
	}

	// Act
	post, err := application.CreatePost(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreatePost: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.posts[post.ID()]
	if !ok {
		t.Fatal("post was not persisted")
	}
	if persisted.Content() != "Bem-vindos ao mural da equipe!" {
		t.Errorf("persisted content = %q, want %q", persisted.Content(), "Bem-vindos ao mural da equipe!")
	}
	if persisted.AuthorID() != input.AuthorID {
		t.Errorf("persisted author id = %v, want %v", persisted.AuthorID(), input.AuthorID)
	}
}

func TestCreatePost_EmptyContent_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreatePostInput{
		TenantID: mustTestTenantID(t),
		AuthorID: domain.NewID(),
		Content:  "   ",
	}

	// Act
	_, err := application.CreatePost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.posts) != 0 {
		t.Errorf("posts = %v, want empty", snapshot.posts)
	}
}

func TestCreatePost_ZeroAuthorID_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreatePostInput{
		TenantID: mustTestTenantID(t),
		AuthorID: domain.ID{},
		Content:  "conteudo valido",
	}

	// Act
	_, err := application.CreatePost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.posts) != 0 {
		t.Errorf("posts = %v, want empty", snapshot.posts)
	}
}

func TestCreatePost_RepositoryFailure_PropagatesError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	uow.errors.postInsert = errs.New(errs.Internal, "db_down", "db unavailable", nil)
	input := application.CreatePostInput{
		TenantID: mustTestTenantID(t),
		AuthorID: domain.NewID(),
		Content:  "conteudo valido",
	}

	// Act
	_, err := application.CreatePost(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Internal) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Internal)
	}
}
