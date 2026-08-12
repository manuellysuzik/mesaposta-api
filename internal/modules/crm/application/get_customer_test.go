package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestGetCustomer_Found_ReturnsCustomer(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	// Act
	got, err := application.GetCustomer(context.Background(), uow, customer.ID())

	// Assert
	if err != nil {
		t.Fatalf("GetCustomer: unexpected error: %v", err)
	}
	if got.ID() != customer.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), customer.ID())
	}
}

func TestGetCustomer_NotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.GetCustomer(context.Background(), uow, mustFakeCustomer(t).ID())

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}
