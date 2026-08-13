package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file exercises fakeUnitOfWork (fakes_test.go) directly, with no
// dependency on any real use case -- proving the fake itself genuinely
// simulates a database transaction's rollback semantics BEFORE any test in
// this package leans on that behavior to prove atomicity of a real use
// case.

const fakeUOWTestTenantID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustFakeUOWTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(fakeUOWTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", fakeUOWTestTenantID, err)
	}
	return id
}

func mustFakeTable(t *testing.T) *domain.Table {
	t.Helper()
	table, err := domain.NewTable(domain.NewID(), mustFakeUOWTenantID(t), "M01", "Mesa 1")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	return &table
}

func TestFakeUnitOfWork_Do_CommitsWritesOnSuccess(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	table := mustFakeTable(t)

	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		return r.Tables().Insert(ctx, table)
	})

	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	got, ok := uow.snapshot().tables[table.ID()]
	if !ok {
		t.Fatal("Do: table was not committed to real state after fn returned nil")
	}
	if got.ID() != table.ID() {
		t.Errorf("committed table ID = %v, want %v", got.ID(), table.ID())
	}
}

func TestFakeUnitOfWork_Do_RollsBackOnError(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	table := mustFakeTable(t)
	sentinel := errors.New("boom")

	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		if insertErr := r.Tables().Insert(ctx, table); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.tables) != 0 {
		t.Errorf("Do: tables = %v, want empty after rollback", snapshot.tables)
	}
}

func TestFakeUnitOfWork_Do_RollbackLeavesPreExistingStateUntouched(t *testing.T) {
	seed := mustFakeTable(t)
	state := newFakeState()
	state.tables[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	sentinel := errors.New("boom")

	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		mutated := *seed
		mutated.Deactivate()
		if updateErr := r.Tables().UpdateWithVersion(ctx, &mutated, seed.Version()); updateErr != nil {
			return updateErr
		}
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	got := uow.snapshot().tables[seed.ID()]
	if got.IsActive() != seed.IsActive() {
		t.Errorf("Do: IsActive() = %v after rollback, want unchanged %v", got.IsActive(), seed.IsActive())
	}
	if got.Version() != seed.Version() {
		t.Errorf("Do: Version() = %d after rollback, want unchanged %d", got.Version(), seed.Version())
	}
}
