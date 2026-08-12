package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetPost_Found_ReturnsPost(t *testing.T) {
	// Arrange
	seed := mustFakePost(t)
	state := newFakeState()
	state.posts[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	// Act
	got, err := application.GetPost(context.Background(), uow, seed.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetPost: unexpected error: %v", err)
	}
	if got.ID() != seed.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), seed.ID())
	}
}

func TestGetPost_NotFound_ReturnsNotFound(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetPost(context.Background(), uow, domain.NewID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
