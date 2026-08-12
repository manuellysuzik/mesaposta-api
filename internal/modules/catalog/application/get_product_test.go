package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetProduct_Found_ReturnsProduct(t *testing.T) {
	// Arrange
	product := mustFakeProduct(t)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)

	// Act
	got, err := application.GetProduct(context.Background(), uow, product.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetProduct: unexpected error: %v", err)
	}
	if got.ID() != product.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), product.ID())
	}
}

func TestGetProduct_NotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetProduct(context.Background(), uow, domain.NewID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
