//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// auditTables lists every table 0010_audit_log.sql introduces. Like
// operationsTables in 0004_operations_rls_integration_test.go, this test
// walks the list confirming each one has both RLS enabled AND forced
// (relrowsecurity/relforcerowsecurity in pg_class) and a
// "tenant_isolation" policy (pg_policies) with the project's exact,
// unchanged-since-0001 predicate.
var auditTables = []string{
	"audit_log",
}

// TestAuditLogMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy
// runs the real migration chain (via db.Migrate, exercised through
// db.TestPostgres) against an ephemeral Postgres container and then
// inspects Postgres' own catalog -- not application code -- to prove every
// table 0010_audit_log.sql creates is actually row-level-security
// protected, not just declared as such in the .sql source.
func TestAuditLogMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, table := range auditTables {
		table := table
		t.Run(table, func(t *testing.T) {
			var relRowSecurity, relForceRowSecurity bool
			err := conn.QueryRow(ctx, `
				SELECT relrowsecurity, relforcerowsecurity
				FROM pg_class
				WHERE relname = $1 AND relkind = 'r'
			`, table).Scan(&relRowSecurity, &relForceRowSecurity)
			if err != nil {
				t.Fatalf("reading pg_class for table %q: %v", table, err)
			}
			if !relRowSecurity {
				t.Errorf("table %q: relrowsecurity = false, want true (ENABLE ROW LEVEL SECURITY)", table)
			}
			if !relForceRowSecurity {
				t.Errorf("table %q: relforcerowsecurity = false, want true (FORCE ROW LEVEL SECURITY)", table)
			}

			var policyCount int
			err = conn.QueryRow(ctx, `
				SELECT count(*)
				FROM pg_policies
				WHERE tablename = $1 AND policyname = 'tenant_isolation'
			`, table).Scan(&policyCount)
			if err != nil {
				t.Fatalf("reading pg_policies for table %q: %v", table, err)
			}
			if policyCount != 1 {
				t.Errorf("table %q: found %d policy(ies) named %q, want exactly 1", table, policyCount, "tenant_isolation")
			}

			var usingExpr, checkExpr string
			err = conn.QueryRow(ctx, `
				SELECT qual, with_check
				FROM pg_policies
				WHERE tablename = $1 AND policyname = 'tenant_isolation'
			`, table).Scan(&usingExpr, &checkExpr)
			if err != nil {
				t.Fatalf("reading tenant_isolation policy predicate for table %q: %v", table, err)
			}
			const wantPredicate = "(tenant_id = (current_setting('app.tenant_id'::text, true))::uuid)"
			if usingExpr != wantPredicate {
				t.Errorf("table %q: USING predicate = %q, want %q", table, usingExpr, wantPredicate)
			}
			if checkExpr != wantPredicate {
				t.Errorf("table %q: WITH CHECK predicate = %q, want %q", table, checkExpr, wantPredicate)
			}
		})
	}
}

// TestAuditLogMigration_GrantsAreAppendOnly asserts, against Postgres'
// own privilege catalog, that mesaposta_app holds exactly INSERT and
// SELECT on audit_log -- and specifically NOT UPDATE or DELETE. This is
// the same append-only discipline stock_movements and transactions follow
// (0004_operations.sql), and it is what makes the audit trail immutable
// by construction rather than by convention: the application role simply
// does not have the privilege to rewrite history.
func TestAuditLogMigration_GrantsAreAppendOnly(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, tc := range []struct {
		privilege string
		want      bool
	}{
		{"SELECT", true},
		{"INSERT", true},
		{"UPDATE", false},
		{"DELETE", false},
	} {
		tc := tc
		t.Run(tc.privilege, func(t *testing.T) {
			var granted bool
			err := conn.QueryRow(ctx,
				`SELECT has_table_privilege('mesaposta_app', 'audit_log', $1)`, tc.privilege).Scan(&granted)
			if err != nil {
				t.Fatalf("checking %s privilege on audit_log: %v", tc.privilege, err)
			}
			if granted != tc.want {
				t.Errorf("has_table_privilege(mesaposta_app, audit_log, %s) = %t, want %t",
					tc.privilege, granted, tc.want)
			}
		})
	}
}
