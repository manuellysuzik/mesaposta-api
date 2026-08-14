//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	auditpg "github.com/manuellysuzik/mesaposta-api/internal/platform/audit/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

func mustEntry(t *testing.T, tenantID tenant.TenantID, actorID, action, entityType, entityID string, occurredAt time.Time, before, after any) audit.Entry {
	t.Helper()
	entry, err := audit.NewEntry(tenantID, actorID, action, entityType, entityID, occurredAt, before, after)
	if err != nil {
		t.Fatalf("audit.NewEntry: %v", err)
	}
	return entry
}

func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	_, appURL := db.TestPostgres(t)
	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRecorder_Record_UsesScopeTenantID_NotEntryTenantID(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantB := mustTenantID(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	// A "forged" entry: constructed with tenant B's id, but recorded
	// through tenant A's context/scope.
	forged := mustEntry(t, tenantB, "user-1", "purchase_order.received", "purchase_order", "po-1", time.Now().UTC(), nil, nil)

	ctxA := tenant.With(context.Background(), tenantA)
	if err := recorder.Record(ctxA, forged); err == nil {
		t.Fatal("Record: want an error for an entry whose TenantID does not match the scope's tenant, got nil")
	}

	// Nothing must have been persisted -- not even under tenant B, and
	// certainly not silently rewritten to tenant A.
	assertAuditCount(t, pool, ctxA, 0, "tenant A after a rejected forged Record")

	ctxB := tenant.With(context.Background(), tenantB)
	assertAuditCount(t, pool, ctxB, 0, "tenant B after a rejected forged Record")
}

func TestRecorder_Record_BatchFailure_PersistsNothing(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "cccccccc-cccc-cccc-cccc-cccccccccccc")
	tenantB := mustTenantID(t, "dddddddd-dddd-dddd-dddd-dddddddddddd")
	ctxA := tenant.With(context.Background(), tenantA)

	now := time.Now().UTC()
	good1 := mustEntry(t, tenantA, "user-1", "purchase_order.received", "purchase_order", "po-1", now, nil, nil)
	good2 := mustEntry(t, tenantA, "user-1", "stock_item.updated", "stock_item", "si-1", now.Add(time.Second), nil, nil)
	// 3rd item: tenant mismatch forces a mid-batch failure.
	bad := mustEntry(t, tenantB, "user-1", "purchase_order.cancelled", "purchase_order", "po-2", now.Add(2*time.Second), nil, nil)

	if err := recorder.Record(ctxA, good1, good2, bad); err == nil {
		t.Fatal("Record: want an error when an item in the batch fails, got nil")
	}

	assertAuditCount(t, pool, ctxA, 0, "after a batch containing a failing item (all-or-nothing)")
}

func TestRecorder_Record_TenantIsolation(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	tenantB := mustTenantID(t, "ffffffff-ffff-ffff-ffff-ffffffffffff")

	ctxA := tenant.With(context.Background(), tenantA)
	entry := mustEntry(t, tenantA, "user-1", "purchase_order.received", "purchase_order", "po-1", time.Now().UTC(), nil, nil)
	if err := recorder.Record(ctxA, entry); err != nil {
		t.Fatalf("Record (tenant A): %v", err)
	}

	ctxB := tenant.With(context.Background(), tenantB)
	assertAuditCount(t, pool, ctxB, 0, "tenant B reading tenant A's audit entries")
}

func TestRecorder_Record_SnapshotsRoundTrip(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "89abcdef-89ab-89ab-89ab-89abcdef0123")
	ctxA := tenant.With(context.Background(), tenantA)

	before := map[string]any{"status": "pendente", "version": float64(1)}
	after := map[string]any{"status": "recebido", "version": float64(2)}
	entry := mustEntry(t, tenantA, "user-42", "purchase_order.received", "purchase_order", "po-1", time.Now().UTC(), before, after)

	if err := recorder.Record(ctxA, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	scope, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctxA)

	var actorID *string
	var rawBefore, rawAfter []byte
	err = scope.QueryRow(ctxA, `
		SELECT actor_id, before_data, after_data
		FROM audit_log
		WHERE action = 'purchase_order.received' AND entity_id = 'po-1'
	`).Scan(&actorID, &rawBefore, &rawAfter)
	if err != nil {
		t.Fatalf("reading back the audit entry: %v", err)
	}

	if actorID == nil || *actorID != "user-42" {
		t.Errorf("actor_id = %v, want %q", actorID, "user-42")
	}

	var gotBefore, gotAfter map[string]any
	if err := json.Unmarshal(rawBefore, &gotBefore); err != nil {
		t.Fatalf("unmarshaling before_data: %v", err)
	}
	if err := json.Unmarshal(rawAfter, &gotAfter); err != nil {
		t.Fatalf("unmarshaling after_data: %v", err)
	}
	if gotBefore["status"] != "pendente" || gotBefore["version"] != float64(1) {
		t.Errorf("before_data = %v, want %v", gotBefore, before)
	}
	if gotAfter["status"] != "recebido" || gotAfter["version"] != float64(2) {
		t.Errorf("after_data = %v, want %v", gotAfter, after)
	}
}

// TestRecorder_Record_EmptyActorAndNilSnapshots_PersistAsSQLNull proves the
// "best-effort actor" design decision (see docs/adr/0006-audit-log.md) is
// real at the storage layer, not just in Go: an action with no
// identifiable human actor stores actor_id as NULL -- never as the empty
// string -- so `WHERE actor_id IS NULL` reliably finds every
// system-performed action.
func TestRecorder_Record_EmptyActorAndNilSnapshots_PersistAsSQLNull(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "01234567-0123-0123-0123-012345678901")
	ctxA := tenant.With(context.Background(), tenantA)

	entry := mustEntry(t, tenantA, "", "stock_item.reconciled", "stock_item", "si-9", time.Now().UTC(), nil, nil)
	if err := recorder.Record(ctxA, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	scope, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctxA)

	var actorNull, beforeNull, afterNull bool
	err = scope.QueryRow(ctxA, `
		SELECT actor_id IS NULL, before_data IS NULL, after_data IS NULL
		FROM audit_log
		WHERE entity_id = 'si-9'
	`).Scan(&actorNull, &beforeNull, &afterNull)
	if err != nil {
		t.Fatalf("reading back the audit entry: %v", err)
	}
	if !actorNull {
		t.Error("actor_id IS NULL = false, want true for an entry with no identifiable actor")
	}
	if !beforeNull {
		t.Error("before_data IS NULL = false, want true for a nil before-snapshot")
	}
	if !afterNull {
		t.Error("after_data IS NULL = false, want true for a nil after-snapshot")
	}
}

// TestAuditLog_IsAppendOnlyForTheApplicationRole is the storage-level
// backstop behind "an audit trail must be immutable": mesaposta_app was
// granted only INSERT and SELECT on audit_log, so even a bug (or a future
// engineer who "just this once" adds an UpdateEntry method) physically
// cannot rewrite or erase a recorded entry.
func TestAuditLog_IsAppendOnlyForTheApplicationRole(t *testing.T) {
	pool := openTestPool(t)
	recorder := auditpg.NewRecorder(pool)

	tenantA := mustTenantID(t, "12345678-1234-1234-1234-123456789012")
	ctxA := tenant.With(context.Background(), tenantA)

	entry := mustEntry(t, tenantA, "user-1", "purchase_order.received", "purchase_order", "po-7", time.Now().UTC(), nil, nil)
	if err := recorder.Record(ctxA, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"UPDATE is not granted", "UPDATE audit_log SET action = 'tampered'"},
		{"DELETE is not granted", "DELETE FROM audit_log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, err := pool.Scope(ctxA)
			if err != nil {
				t.Fatalf("Scope: %v", err)
			}
			defer scope.Rollback(ctxA)

			if _, err := scope.Exec(ctxA, tc.sql); err == nil {
				t.Fatalf("%q succeeded, want a permission-denied error (audit_log must be append-only for mesaposta_app)", tc.sql)
			}
		})
	}
}

// assertAuditCount asserts how many audit_log rows are visible to ctx's
// tenant.
func assertAuditCount(t *testing.T, pool *db.Pool, ctx context.Context, want int, what string) {
	t.Helper()

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (%s): %v", what, err)
	}
	defer scope.Rollback(ctx)

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&count); err != nil {
		t.Fatalf("counting audit entries (%s): %v", what, err)
	}
	if count != want {
		t.Fatalf("%s: found %d audit entry(ies), want %d", what, count, want)
	}
}
