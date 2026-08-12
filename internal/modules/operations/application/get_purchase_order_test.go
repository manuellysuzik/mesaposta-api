package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetPurchaseOrder_Found_ReturnsOrder(t *testing.T) {
	// Arrange
	f := newReceiveFixture(t)

	// Act
	got, err := application.GetPurchaseOrder(context.Background(), f.uow, f.order.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetPurchaseOrder: unexpected error: %v", err)
	}
	if got.ID() != f.order.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), f.order.ID())
	}
}

func TestGetPurchaseOrder_NotFound_ReturnsNotFound(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetPurchaseOrder(context.Background(), uow, domain.NewID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
