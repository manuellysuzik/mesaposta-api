package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
)

func TestDeactivateTable_ThenActivateTable_RoundTrips(t *testing.T) {
	table := mustFakeTable(t)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)

	deactivated, err := application.DeactivateTable(context.Background(), uow, table.ID())
	if err != nil {
		t.Fatalf("DeactivateTable: unexpected error: %v", err)
	}
	if deactivated.IsActive() {
		t.Error("IsActive() = true after DeactivateTable, want false")
	}
	if deactivated.Version() != table.Version()+1 {
		t.Errorf("Version() = %d, want %d", deactivated.Version(), table.Version()+1)
	}

	activated, err := application.ActivateTable(context.Background(), uow, table.ID())
	if err != nil {
		t.Fatalf("ActivateTable: unexpected error: %v", err)
	}
	if !activated.IsActive() {
		t.Error("IsActive() = false after ActivateTable, want true")
	}
	if activated.Version() != deactivated.Version()+1 {
		t.Errorf("Version() = %d, want %d", activated.Version(), deactivated.Version()+1)
	}
}

// TestActivateTable_AlreadyActive_IsIdempotentAndSkipsPersistence proves the
// idempotency documented on domain.Table.Activate holds all the way through
// the application layer: a redundant ActivateTable call must not advance
// Version, and (per mutateTable's own doc comment) must not even issue an
// UpdateWithVersion call -- proven here by injecting a canned error into
// tableUpdateWithVersion and confirming it is never triggered.
func TestActivateTable_AlreadyActive_IsIdempotentAndSkipsPersistence(t *testing.T) {
	table := mustFakeTable(t)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)
	uow.errors.tableUpdateWithVersion = errUnexpectedPersistCall

	got, err := application.ActivateTable(context.Background(), uow, table.ID())
	if err != nil {
		t.Fatalf("ActivateTable: unexpected error: %v", err)
	}
	if got.Version() != table.Version() {
		t.Errorf("Version() = %d, want unchanged %d", got.Version(), table.Version())
	}
}
