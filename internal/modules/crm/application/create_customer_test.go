package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreateCustomer_Valid_PersistsCustomer(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	email := "cliente@example.com"
	input := application.CreateCustomerInput{
		TenantID: mustFakeUOWTenantID(t),
		Name:     "Cliente Teste",
		Email:    &email,
	}

	// Act
	customer, err := application.CreateCustomer(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreateCustomer: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.customers[customer.ID()]
	if !ok {
		t.Fatal("customer was not persisted")
	}
	if persisted.Name() != "Cliente Teste" {
		t.Errorf("persisted name = %q, want %q", persisted.Name(), "Cliente Teste")
	}
	if persisted.LoyaltyPoints().Points() != 0 {
		t.Errorf("persisted loyalty points = %d, want 0", persisted.LoyaltyPoints().Points())
	}
}

func TestCreateCustomer_InvalidName_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateCustomerInput{
		TenantID: mustFakeUOWTenantID(t),
		Name:     "",
	}

	// Act
	_, err := application.CreateCustomer(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.customers) != 0 {
		t.Errorf("customers = %v, want empty", snapshot.customers)
	}
}

func TestCreateCustomer_DuplicateContact_PropagatesConflict(t *testing.T) {
	// Arrange: simulate the database's unique-per-tenant constraint firing
	// on Insert (see internal/platform/db/migrations/0006_crm.sql).
	uow := newFakeUnitOfWork(newFakeState())
	uow.errors.customerInsert = errs.New(errs.Conflict, "customer_duplicate_contact",
		"a customer with this email or phone already exists for this tenant", nil)

	input := application.CreateCustomerInput{
		TenantID: mustFakeUOWTenantID(t),
		Name:     "Cliente Duplicado",
	}

	// Act
	_, err := application.CreateCustomer(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
}
