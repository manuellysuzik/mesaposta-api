package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreateRecipe_Valid_PersistsRecipe(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateRecipeInput{
		TenantID: mustFakeUOWTenantID(t),
		Name:     "Molho especial",
	}

	// Act
	recipe, err := application.CreateRecipe(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreateRecipe: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.recipes[recipe.ID()]
	if !ok {
		t.Fatal("recipe was not persisted")
	}
	if persisted.Name() != "Molho especial" {
		t.Errorf("persisted name = %q, want %q", persisted.Name(), "Molho especial")
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty (CreateRecipe does not publish an event)", snapshot.events)
	}
}

func TestCreateRecipe_EmptyName_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateRecipeInput{
		TenantID: mustFakeUOWTenantID(t),
		Name:     "",
	}

	// Act
	_, err := application.CreateRecipe(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.recipes) != 0 {
		t.Errorf("recipes = %v, want empty", snapshot.recipes)
	}
}
