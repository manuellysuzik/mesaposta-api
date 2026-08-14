package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreditLoyaltyPoints_Valid_UpdatesBalanceAndAppendsEntry(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	// Act
	err := application.CreditLoyaltyPoints(context.Background(), uow, application.CreditLoyaltyPointsInput{
		CustomerID: customer.ID(),
		Points:     mustFakeLoyaltyPoints(t, 50),
		Reason:     "Pedido #1",
	}, "user-1")

	// Assert
	if err != nil {
		t.Fatalf("CreditLoyaltyPoints: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted := snapshot.customers[customer.ID()]
	if persisted.LoyaltyPoints().Points() != 50 {
		t.Errorf("persisted.LoyaltyPoints().Points() = %d, want 50", persisted.LoyaltyPoints().Points())
	}
	if persisted.Version() != customer.Version()+1 {
		t.Errorf("persisted.Version() = %d, want %d", persisted.Version(), customer.Version()+1)
	}
	if len(snapshot.loyaltyEntries) != 1 {
		t.Fatalf("loyaltyEntries = %d, want 1", len(snapshot.loyaltyEntries))
	}
	entry := snapshot.loyaltyEntries[0]
	if entry.Points().Points() != 50 {
		t.Errorf("entry.Points().Points() = %d, want 50", entry.Points().Points())
	}
	if len(snapshot.events) != 1 {
		t.Errorf("events = %d, want 1", len(snapshot.events))
	}
	if len(snapshot.auditEntries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(snapshot.auditEntries))
	}
	auditEntry := snapshot.auditEntries[0]
	if auditEntry.ActorID != "user-1" {
		t.Errorf("auditEntry.ActorID = %q, want %q", auditEntry.ActorID, "user-1")
	}
	if auditEntry.Action != "loyalty_entry.credited" {
		t.Errorf("auditEntry.Action = %q, want %q", auditEntry.Action, "loyalty_entry.credited")
	}
}

func TestCreditLoyaltyPoints_CustomerNotFound_ReturnsNotFoundAndPersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	err := application.CreditLoyaltyPoints(context.Background(), uow, application.CreditLoyaltyPointsInput{
		CustomerID: mustFakeCustomer(t).ID(),
		Points:     mustFakeLoyaltyPoints(t, 50),
		Reason:     "Pedido #1",
	}, "user-1")

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}

func TestCreditLoyaltyPoints_ZeroPoints_PersistsNothing(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	// Act
	err := application.CreditLoyaltyPoints(context.Background(), uow, application.CreditLoyaltyPointsInput{
		CustomerID: customer.ID(),
		Points:     mustFakeLoyaltyPoints(t, 0),
		Reason:     "Pedido #1",
	}, "user-1")

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	snapshot := uow.snapshot()
	if snapshot.customers[customer.ID()].LoyaltyPoints().Points() != 0 {
		t.Errorf("balance mutated on rejected credit: %d, want unchanged 0", snapshot.customers[customer.ID()].LoyaltyPoints().Points())
	}
	if len(snapshot.loyaltyEntries) != 0 {
		t.Errorf("loyaltyEntries = %d, want 0", len(snapshot.loyaltyEntries))
	}
	if len(snapshot.auditEntries) != 0 {
		t.Errorf("audit entries = %v, want none: the invariant fails BEFORE any write is attempted", snapshot.auditEntries)
	}
}
