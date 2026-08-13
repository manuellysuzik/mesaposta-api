//go:build integration

package migrations_test

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// TestTenantsMigration_RLSEnabledAndForced_WithPermissiveSelectPolicy runs
// the real migration chain (via db.Migrate, exercised through
// db.TestPostgres) against an ephemeral Postgres container and then
// inspects Postgres' own catalog -- not application code -- to prove
// 0008_tenants.sql's deliberate departure from the "tenant_isolation"
// convention: RLS is still ENABLE+FORCE, but the policy is a permissive
// USING (true), matching this project's other tables in posture while
// differing in predicate for the documented reason (see
// 0008_tenants.sql's header comment).
func TestTenantsMigration_RLSEnabledAndForced_WithPermissiveSelectPolicy(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	var relRowSecurity, relForceRowSecurity bool
	err = conn.QueryRow(ctx, `
		SELECT relrowsecurity, relforcerowsecurity
		FROM pg_class
		WHERE relname = 'tenants' AND relkind = 'r'
	`).Scan(&relRowSecurity, &relForceRowSecurity)
	if err != nil {
		t.Fatalf("reading pg_class for table \"tenants\": %v", err)
	}
	if !relRowSecurity {
		t.Errorf("table \"tenants\": relrowsecurity = false, want true (ENABLE ROW LEVEL SECURITY)")
	}
	if !relForceRowSecurity {
		t.Errorf("table \"tenants\": relforcerowsecurity = false, want true (FORCE ROW LEVEL SECURITY)")
	}

	var policyCount int
	err = conn.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'tenants' AND policyname = 'public_slug_lookup'
	`).Scan(&policyCount)
	if err != nil {
		t.Fatalf("reading pg_policies for table \"tenants\": %v", err)
	}
	if policyCount != 1 {
		t.Errorf("table \"tenants\": found %d policy(ies) named %q, want exactly 1", policyCount, "public_slug_lookup")
	}

	var usingExpr *string
	err = conn.QueryRow(ctx, `
		SELECT qual
		FROM pg_policies
		WHERE tablename = 'tenants' AND policyname = 'public_slug_lookup'
	`).Scan(&usingExpr)
	if err != nil {
		t.Fatalf("reading public_slug_lookup policy predicate: %v", err)
	}
	if usingExpr == nil || *usingExpr != "true" {
		got := "<nil>"
		if usingExpr != nil {
			got = *usingExpr
		}
		t.Errorf("table \"tenants\": USING predicate = %q, want %q", got, "true")
	}
}

// TestTenantsMigration_ColumnGrant_LimitedToIDSlugIsActive proves
// mesaposta_app's SELECT grant on tenants is a column-level grant limited
// to exactly {id, slug, is_active} -- never a blanket table-level SELECT
// -- so any future, more sensitive column added to this table is not
// automatically exposed through the same runtime credential public,
// unauthenticated slug lookups use.
func TestTenantsMigration_ColumnGrant_LimitedToIDSlugIsActive(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT column_name
		FROM information_schema.column_privileges
		WHERE grantee = 'mesaposta_app' AND table_name = 'tenants' AND privilege_type = 'SELECT'
	`)
	if err != nil {
		t.Fatalf("reading column_privileges for table \"tenants\": %v", err)
	}
	var gotColumns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scanning column_privileges row: %v", err)
		}
		gotColumns = append(gotColumns, col)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating column_privileges rows: %v", err)
	}
	sort.Strings(gotColumns)

	wantColumns := []string{"id", "is_active", "slug"}
	if len(gotColumns) != len(wantColumns) {
		t.Fatalf("mesaposta_app SELECT column grants = %v, want %v", gotColumns, wantColumns)
	}
	for i, want := range wantColumns {
		if gotColumns[i] != want {
			t.Errorf("mesaposta_app SELECT column grants = %v, want %v", gotColumns, wantColumns)
			break
		}
	}
}

// TestTenantsMigration_NoWriteGrantsForAppRole proves mesaposta_app has NO
// INSERT/UPDATE/DELETE privilege on tenants at all: provisioning a tenant
// is deliberately an administrative operation outside the application's
// runtime request path (see 0008_tenants.sql's header comment).
func TestTenantsMigration_NoWriteGrantsForAppRole(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, privilege := range []string{"INSERT", "UPDATE", "DELETE"} {
		var hasPrivilege bool
		err := conn.QueryRow(ctx, `SELECT has_table_privilege('mesaposta_app', 'tenants', $1)`, privilege).
			Scan(&hasPrivilege)
		if err != nil {
			t.Fatalf("checking has_table_privilege(mesaposta_app, tenants, %q): %v", privilege, err)
		}
		if hasPrivilege {
			t.Errorf("mesaposta_app has %s privilege on \"tenants\", want none", privilege)
		}
	}
}
