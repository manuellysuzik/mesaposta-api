package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreateTable_Valid_PersistsAndReturnsActiveTable(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())

	table, err := application.CreateTable(context.Background(), uow, application.CreateTableInput{
		TenantID: mustFakeUOWTenantID(t), Code: "M01", Label: "Mesa 1",
	})
	if err != nil {
		t.Fatalf("CreateTable: unexpected error: %v", err)
	}
	if !table.IsActive() {
		t.Error("IsActive() = false, want true")
	}
	if _, ok := uow.snapshot().tables[table.ID()]; !ok {
		t.Error("table was not persisted")
	}
}

func TestCreateTable_BlankCode_ReturnsInvalidAndPersistsNothing(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())

	_, err := application.CreateTable(context.Background(), uow, application.CreateTableInput{
		TenantID: mustFakeUOWTenantID(t), Code: "  ", Label: "Mesa 1",
	})
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
	if len(uow.snapshot().tables) != 0 {
		t.Error("CreateTable persisted a table despite validation failure")
	}
}

func TestGetTable_Found(t *testing.T) {
	table := mustFakeTable(t)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)

	got, err := application.GetTable(context.Background(), uow, table.ID())
	if err != nil {
		t.Fatalf("GetTable: unexpected error: %v", err)
	}
	if got.ID() != table.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), table.ID())
	}
}

func TestGetTable_NotFound(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())

	_, err := application.GetTable(context.Background(), uow, domain.NewID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestListTables_ReturnsEveryTable(t *testing.T) {
	state := newFakeState()
	t1 := mustFakeTable(t)
	state.tables[t1.ID()] = t1
	uow := newFakeUnitOfWork(state)

	got, err := application.ListTables(context.Background(), uow)
	if err != nil {
		t.Fatalf("ListTables: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}
