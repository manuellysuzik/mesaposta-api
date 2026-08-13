package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustTable(t *testing.T, code, label string) domain.Table {
	t.Helper()
	table, err := domain.NewTable(domain.NewID(), mustOrderTenantID(t), code, label)
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	return table
}

func TestNewTable_BlankCode_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewTable(domain.NewID(), mustOrderTenantID(t), "   ", "Mesa 1")
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewTable_BlankLabel_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewTable(domain.NewID(), mustOrderTenantID(t), "M01", "")
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewTable_Valid_StartsActiveVersion1(t *testing.T) {
	table := mustTable(t, "M01", "Mesa 1")

	if !table.IsActive() {
		t.Error("IsActive() = false, want true")
	}
	if table.Version() != 1 {
		t.Errorf("Version() = %d, want 1", table.Version())
	}
	if table.Code() != "M01" {
		t.Errorf("Code() = %q, want %q", table.Code(), "M01")
	}
}

func TestTable_Deactivate_Activate(t *testing.T) {
	table := mustTable(t, "M01", "Mesa 1")

	table.Deactivate()
	if table.IsActive() {
		t.Error("IsActive() = true after Deactivate, want false")
	}
	if table.Version() != 2 {
		t.Errorf("Version() = %d after Deactivate, want 2", table.Version())
	}

	table.Activate()
	if !table.IsActive() {
		t.Error("IsActive() = false after Activate, want true")
	}
	if table.Version() != 3 {
		t.Errorf("Version() = %d after Activate, want 3", table.Version())
	}
}

func TestTable_Activate_AlreadyActive_IsIdempotentNoOp(t *testing.T) {
	table := mustTable(t, "M01", "Mesa 1")
	versionBefore := table.Version()

	table.Activate()

	if table.Version() != versionBefore {
		t.Errorf("Version() = %d after redundant Activate, want unchanged %d", table.Version(), versionBefore)
	}
}

func TestTable_Deactivate_AlreadyInactive_IsIdempotentNoOp(t *testing.T) {
	table := mustTable(t, "M01", "Mesa 1")
	table.Deactivate()
	versionBefore := table.Version()

	table.Deactivate()

	if table.Version() != versionBefore {
		t.Errorf("Version() = %d after redundant Deactivate, want unchanged %d", table.Version(), versionBefore)
	}
}
