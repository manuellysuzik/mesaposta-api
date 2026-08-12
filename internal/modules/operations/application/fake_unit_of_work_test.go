package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file exercises fakeUnitOfWork (fakes_test.go) directly, with no
// dependency on any real use case -- proving the fake itself genuinely
// simulates a database transaction's rollback semantics BEFORE any test in
// this package leans on that behavior to prove atomicity of a real use
// case (see receive_purchase_order_test.go). Without this guarantee, every
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

func mustFakeStockItem(t *testing.T) *domain.StockItem {
	t.Helper()
	qty, err := domain.NewQuantity(1000)
	if err != nil {
		t.Fatalf("NewQuantity: %v", err)
	}
	zero, err := domain.NewQuantity(0)
	if err != nil {
		t.Fatalf("NewQuantity(0): %v", err)
	}
	cost, err := domain.NewMoney(100)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	item, err := domain.NewStockItem(
		domain.NewID(),
		mustFakeUOWTenantID(t),
		"Farinha de trigo",
		domain.StockUnitKg,
		qty,
		zero,
		cost,
		nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}
	return &item
}

func TestFakeUnitOfWork_Do_CommitsWritesOnSuccess(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	item := mustFakeStockItem(t)

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		return r.StockItems().Insert(ctx, item)
	})

	// Assert
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	got, findErr := uow.snapshot().stockItems[item.ID()]
	if !findErr {
		t.Fatal("Do: item was not committed to real state after fn returned nil")
	}
	if got.ID() != item.ID() {
		t.Errorf("committed item ID = %v, want %v", got.ID(), item.ID())
	}
}

func TestFakeUnitOfWork_Do_RollsBackOnError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	item := mustFakeStockItem(t)
	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		if insertErr := r.StockItems().Insert(ctx, item); insertErr != nil {
			return insertErr
		}
		// A second write, past the first, to prove EVERYTHING attempted
		// within this failed Do call is discarded -- not just the last
		// write before the error.
		movement, movementErr := domain.NewStockMovement(
			domain.NewID(),
			mustFakeUOWTenantID(t),
			item.ID(),
			domain.StockMovementTypeEntrada,
			mustFakeQuantity(t, 1000),
			"test movement",
		)
		if movementErr != nil {
			return movementErr
		}
		if insertErr := r.StockMovements().Insert(ctx, movement); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.stockItems) != 0 {
		t.Errorf("Do: stockItems = %v, want empty after rollback", snapshot.stockItems)
	}
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("Do: stockMovements = %v, want empty after rollback", snapshot.stockMovements)
	}
}

func TestFakeUnitOfWork_Do_RollbackLeavesPreExistingStateUntouched(t *testing.T) {
	// Arrange: seed the fake with one already-committed stock item, then
	// run a failing Do call that tries to mutate it.
	seed := mustFakeStockItem(t)
	state := newFakeState()
	state.stockItems[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		mutated := *seed
		if withdrawErr := mutated.Withdraw(mustFakeQuantity(t, 500)); withdrawErr != nil {
			return withdrawErr
		}
		if updateErr := r.StockItems().UpdateWithVersion(ctx, &mutated, seed.Version()); updateErr != nil {
			return updateErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	got := uow.snapshot().stockItems[seed.ID()]
	if got.Quantity() != seed.Quantity() {
		t.Errorf("Do: quantity = %v after rollback, want unchanged %v", got.Quantity(), seed.Quantity())
	}
	if got.Version() != seed.Version() {
		t.Errorf("Do: version = %d after rollback, want unchanged %d", got.Version(), seed.Version())
	}
}

func mustFakeQuantity(t *testing.T, thousandths int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(thousandths)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", thousandths, err)
	}
	return q
}
