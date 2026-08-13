package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestResolveTableForMenu_ActiveTable_MintsSessionAndPersistsIt(t *testing.T) {
	table := mustFakeTable(t)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)

	result, err := application.ResolveTableForMenu(context.Background(), uow, mustFakeUOWTenantID(t), table.Code())
	if err != nil {
		t.Fatalf("ResolveTableForMenu: unexpected error: %v", err)
	}
	if result.SessionToken == "" {
		t.Fatal("SessionToken is empty")
	}
	if result.Table.ID() != table.ID() {
		t.Errorf("result.Table.ID() = %v, want %v", result.Table.ID(), table.ID())
	}
	if _, ok := uow.snapshot().tableSessions[result.Session.TokenHash()]; !ok {
		t.Error("table session was not persisted")
	}
}

// TestResolveTableForMenu_UnknownAndInactiveCode_ProduceIdenticalErrors is
// this module's headline security requirement, proven at the
// application-layer error shape (see ../infrastructure/httppublic's own
// handler-level test for the byte-for-byte HTTP response proof): an
// unknown table code and a known-but-inactive table code must be
// COMPLETELY indistinguishable to the caller -- same Kind, same Code, same
// Public message.
func TestResolveTableForMenu_UnknownAndInactiveCode_ProduceIdenticalErrors(t *testing.T) {
	inactiveTable := mustFakeTable(t)
	inactiveTable.Deactivate()
	state := newFakeState()
	state.tables[inactiveTable.ID()] = inactiveTable
	uow := newFakeUnitOfWork(state)

	_, unknownErr := application.ResolveTableForMenu(context.Background(), uow, mustFakeUOWTenantID(t), "does-not-exist")
	_, inactiveErr := application.ResolveTableForMenu(context.Background(), uow, mustFakeUOWTenantID(t), inactiveTable.Code())

	if !errs.IsKind(unknownErr, errs.NotFound) {
		t.Fatalf("unknownErr Kind = %v, want errs.NotFound", errs.KindOf(unknownErr))
	}
	if !errs.IsKind(inactiveErr, errs.NotFound) {
		t.Fatalf("inactiveErr Kind = %v, want errs.NotFound", errs.KindOf(inactiveErr))
	}

	var structuredUnknown, structuredInactive *errs.Error
	if !asStructuredError(unknownErr, &structuredUnknown) || !asStructuredError(inactiveErr, &structuredInactive) {
		t.Fatal("both errors must be *errs.Error")
	}
	if structuredUnknown.Code() != structuredInactive.Code() {
		t.Errorf("Code() differs: unknown=%q inactive=%q", structuredUnknown.Code(), structuredInactive.Code())
	}
	if structuredUnknown.Public() != structuredInactive.Public() {
		t.Errorf("Public() differs: unknown=%q inactive=%q", structuredUnknown.Public(), structuredInactive.Public())
	}
}

func asStructuredError(err error, target **errs.Error) bool {
	e, ok := err.(*errs.Error)
	if !ok {
		return false
	}
	*target = e
	return true
}

func TestResolveTableForMenu_ActiveTable_DoesNotErrorWhenSessionInsertSucceeds(t *testing.T) {
	table := mustFakeTable(t)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)

	_, err := application.ResolveTableForMenu(context.Background(), uow, mustFakeUOWTenantID(t), table.Code())
	if err != nil {
		t.Fatalf("ResolveTableForMenu: unexpected error: %v", err)
	}
}
