package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
)

func TestListActiveProducts_ReturnsOnlyActiveProductsOrderedByName(t *testing.T) {
	// Arrange
	state := newFakeState()

	burger := mustFakeProduct(t)
	state.products[burger.ID()] = burger

	soda := mustFakeProductNamed(t, "Refrigerante")
	state.products[soda.ID()] = soda

	uow := newFakeUnitOfWork(state)

	// Act
	got, err := application.ListActiveProducts(context.Background(), uow)

	// Assert
	if err != nil {
		t.Fatalf("ListActiveProducts: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	// "Refrigerante" < "X-Burguer" alphabetically.
	if got[0].Name() != "Refrigerante" {
		t.Errorf("got[0].Name() = %q, want %q", got[0].Name(), "Refrigerante")
	}
	if got[1].Name() != "X-Burguer" {
		t.Errorf("got[1].Name() = %q, want %q", got[1].Name(), "X-Burguer")
	}
}

func TestListActiveProducts_NoProducts_ReturnsEmpty(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	got, err := application.ListActiveProducts(context.Background(), uow)

	// Assert
	if err != nil {
		t.Fatalf("ListActiveProducts: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
