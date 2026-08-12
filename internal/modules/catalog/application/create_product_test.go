package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreateProduct_Valid_PersistsProductAndPublishesEvent(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateProductInput{
		TenantID:  mustFakeUOWTenantID(t),
		Name:      "X-Burguer",
		BasePrice: mustFakeMoney(t, 1500),
	}

	// Act
	product, err := application.CreateProduct(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreateProduct: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.products[product.ID()]
	if !ok {
		t.Fatal("product was not persisted")
	}
	if persisted.Name() != "X-Burguer" {
		t.Errorf("persisted name = %q, want %q", persisted.Name(), "X-Burguer")
	}
	if len(snapshot.events) != 1 {
		t.Fatalf("events published = %d, want 1", len(snapshot.events))
	}
}

func TestCreateProduct_InvalidName_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateProductInput{
		TenantID:  mustFakeUOWTenantID(t),
		Name:      "",
		BasePrice: mustFakeMoney(t, 1500),
	}

	// Act
	_, err := application.CreateProduct(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.products) != 0 {
		t.Errorf("products = %v, want empty", snapshot.products)
	}
}

func TestCreateProduct_ZeroPrice_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateProductInput{
		TenantID:  mustFakeUOWTenantID(t),
		Name:      "Refrigerante",
		BasePrice: mustFakeMoney(t, 0),
	}

	// Act
	_, err := application.CreateProduct(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.products) != 0 {
		t.Errorf("products = %v, want empty", snapshot.products)
	}
}

func TestCreateProduct_InsertFails_PublishesNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	uow.errors.productInsert = errs.New(errs.Internal, "boom", "boom", nil)
	input := application.CreateProductInput{
		TenantID:  mustFakeUOWTenantID(t),
		Name:      "X-Burguer",
		BasePrice: mustFakeMoney(t, 1500),
	}

	// Act
	_, err := application.CreateProduct(context.Background(), uow, input)

	// Assert
	if err == nil {
		t.Fatal("CreateProduct returned nil error, want an error")
	}
	if snapshot := uow.snapshot(); len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty (nothing published after a failed insert)", snapshot.events)
	}
}
