package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestRedeemLoyaltyPoints_Valid_UpdatesBalanceAndAppendsEntry(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	if err := customer.CreditPoints(mustFakeLoyaltyPoints(t, 100)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	versionBefore := customer.Version()

	// Act
	err := application.RedeemLoyaltyPoints(context.Background(), uow, application.RedeemLoyaltyPointsInput{
		CustomerID: customer.ID(),
		Points:     mustFakeLoyaltyPoints(t, 30),
		Reason:     "Resgate de brinde",
	})

	// Assert
	if err != nil {
		t.Fatalf("RedeemLoyaltyPoints: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted := snapshot.customers[customer.ID()]
	if persisted.LoyaltyPoints().Points() != 70 {
		t.Errorf("persisted.LoyaltyPoints().Points() = %d, want 70", persisted.LoyaltyPoints().Points())
	}
	if persisted.Version() != versionBefore+1 {
		t.Errorf("persisted.Version() = %d, want %d", persisted.Version(), versionBefore+1)
	}
	if len(snapshot.loyaltyEntries) != 1 {
		t.Fatalf("loyaltyEntries = %d, want 1", len(snapshot.loyaltyEntries))
	}
	if len(snapshot.events) != 1 {
		t.Errorf("events = %d, want 1", len(snapshot.events))
	}
}

// TestRedeemLoyaltyPoints_InsufficientBalance_ReturnsConflictAndPersistsNothing
// is this module's headline acceptance criterion, proven once more at the
// application layer: redeeming more points than the customer's balance
// must be rejected with Kind errs.Conflict, and -- because
// Customer.RedeemPoints fails BEFORE any repository write is attempted --
// absolutely nothing may be persisted: not the customer's balance/version,
// not a LoyaltyEntry, not a domain event.
func TestRedeemLoyaltyPoints_InsufficientBalance_ReturnsConflictAndPersistsNothing(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	if err := customer.CreditPoints(mustFakeLoyaltyPoints(t, 20)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	balanceBefore := customer.LoyaltyPoints()
	versionBefore := customer.Version()

	// Act: try to redeem more than the 20-point balance.
	err := application.RedeemLoyaltyPoints(context.Background(), uow, application.RedeemLoyaltyPointsInput{
		CustomerID: customer.ID(),
		Points:     mustFakeLoyaltyPoints(t, 50),
		Reason:     "Resgate de brinde caro demais",
	})

	// Assert: the error.
	if err == nil {
		t.Fatal("RedeemLoyaltyPoints(more than balance) returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	// Assert: nothing was persisted at all.
	snapshot := uow.snapshot()
	persisted := snapshot.customers[customer.ID()]
	if persisted.LoyaltyPoints() != balanceBefore {
		t.Errorf("persisted.LoyaltyPoints() = %v, want unchanged %v", persisted.LoyaltyPoints(), balanceBefore)
	}
	if persisted.Version() != versionBefore {
		t.Errorf("persisted.Version() = %d, want unchanged %d", persisted.Version(), versionBefore)
	}
	if len(snapshot.loyaltyEntries) != 0 {
		t.Errorf("loyaltyEntries = %d, want 0 (redemption must have been rejected before any write)", len(snapshot.loyaltyEntries))
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %d, want 0", len(snapshot.events))
	}
}

func TestRedeemLoyaltyPoints_CustomerNotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	err := application.RedeemLoyaltyPoints(context.Background(), uow, application.RedeemLoyaltyPointsInput{
		CustomerID: mustFakeCustomer(t).ID(),
		Points:     mustFakeLoyaltyPoints(t, 10),
		Reason:     "Resgate",
	})

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}

func TestRedeemLoyaltyPoints_ZeroPoints_ReturnsInvalidError(t *testing.T) {
	// Arrange
	customer := mustFakeCustomer(t)
	if err := customer.CreditPoints(mustFakeLoyaltyPoints(t, 20)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	// Act
	err := application.RedeemLoyaltyPoints(context.Background(), uow, application.RedeemLoyaltyPointsInput{
		CustomerID: customer.ID(),
		Points:     mustFakeLoyaltyPoints(t, 0),
		Reason:     "Resgate",
	})

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}
