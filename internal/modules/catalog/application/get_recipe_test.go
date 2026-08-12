package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetRecipe_Found_ReturnsRecipe(t *testing.T) {
	// Arrange
	recipe := mustFakeRecipe(t, nil)
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)

	// Act
	got, err := application.GetRecipe(context.Background(), uow, recipe.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetRecipe: unexpected error: %v", err)
	}
	if got.ID() != recipe.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), recipe.ID())
	}
}

func TestGetRecipe_NotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetRecipe(context.Background(), uow, domain.NewID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
