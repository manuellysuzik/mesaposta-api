package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetStockItem_Found_ReturnsItem(t *testing.T) {
	// Arrange
	uow, item := newStockItemFixture(t, 1000)

	// Act
	got, err := application.GetStockItem(context.Background(), uow, item.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetStockItem: unexpected error: %v", err)
	}
	if got.ID() != item.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), item.ID())
	}
}

func TestGetStockItem_NotFound_ReturnsNotFound(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetStockItem(context.Background(), uow, domain.NewID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
