package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file exercises fakeUnitOfWork (fakes_test.go) directly, with no
// dependency on any real use case -- proving the fake itself genuinely
// simulates a database transaction's rollback semantics BEFORE any test in
// this package leans on that behavior to prove atomicity of a real use
// case (see redeem_loyalty_points_test.go). Without this guarantee, every
// "nothing was persisted after an error" assertion elsewhere in this
// package would be testing nothing at all.

const fakeUOWTestTenantID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustFakeUOWTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(fakeUOWTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", fakeUOWTestTenantID, err)
	}
	return id
}

func mustFakeCustomer(t *testing.T) *domain.Customer {
	t.Helper()
	customer, err := domain.NewCustomer(domain.NewID(), mustFakeUOWTenantID(t), "Cliente Teste", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	return &customer
}

func mustFakeLoyaltyPoints(t *testing.T, points int64) domain.LoyaltyPoints {
	t.Helper()
	p, err := domain.NewLoyaltyPoints(points)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(%d): %v", points, err)
	}
	return p
}

func TestFakeUnitOfWork_Do_CommitsWritesOnSuccess(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	customer := mustFakeCustomer(t)

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		return r.Customers().Insert(ctx, customer)
	})

	// Assert
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	got, ok := uow.snapshot().customers[customer.ID()]
	if !ok {
		t.Fatal("Do: customer was not committed to real state after fn returned nil")
	}
	if got.ID() != customer.ID() {
		t.Errorf("committed customer ID = %v, want %v", got.ID(), customer.ID())
	}
}

func TestFakeUnitOfWork_Do_RollsBackOnError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	customer := mustFakeCustomer(t)
	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		if insertErr := r.Customers().Insert(ctx, customer); insertErr != nil {
			return insertErr
		}
		// A second write, past the first, to prove EVERYTHING attempted
		// within this failed Do call is discarded -- not just the last
		// write before the error.
		entry, entryErr := domain.NewLoyaltyEntry(
			domain.NewID(), mustFakeUOWTenantID(t), customer.ID(),
			domain.LoyaltyEntryTypeCredito, mustFakeLoyaltyPoints(t, 10), "test entry",
		)
		if entryErr != nil {
			return entryErr
		}
		if insertErr := r.LoyaltyEntries().Insert(ctx, entry); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.customers) != 0 {
		t.Errorf("Do: customers = %v, want empty after rollback", snapshot.customers)
	}
	if len(snapshot.loyaltyEntries) != 0 {
		t.Errorf("Do: loyaltyEntries = %v, want empty after rollback", snapshot.loyaltyEntries)
	}
}

func TestFakeUnitOfWork_Do_RollbackLeavesPreExistingStateUntouched(t *testing.T) {
	// Arrange: seed the fake with one already-committed customer, then
	// run a failing Do call that tries to mutate it.
	seed := mustFakeCustomer(t)
	if err := seed.CreditPoints(mustFakeLoyaltyPoints(t, 100)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		mutated := *seed
		if redeemErr := mutated.RedeemPoints(mustFakeLoyaltyPoints(t, 50)); redeemErr != nil {
			return redeemErr
		}
		if updateErr := r.Customers().UpdateWithVersion(ctx, &mutated, seed.Version()); updateErr != nil {
			return updateErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	got := uow.snapshot().customers[seed.ID()]
	if got.LoyaltyPoints() != seed.LoyaltyPoints() {
		t.Errorf("Do: loyalty points = %v after rollback, want unchanged %v", got.LoyaltyPoints(), seed.LoyaltyPoints())
	}
	if got.Version() != seed.Version() {
		t.Errorf("Do: version = %d after rollback, want unchanged %d", got.Version(), seed.Version())
	}
}
