//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// orderingTables lists every table 0009_ordering.sql introduces -- this
// test walks this exact list, confirming each one has both RLS enabled AND
// forced (relrowsecurity/relforcerowsecurity in pg_class) and a
// "tenant_isolation" policy (pg_policies), the same way every other
// module's own migration (0001-0007) already does. Unlike 0008_tenants.sql
// (the one deliberate exception in this schema), every table below follows
// that normal convention -- see 0009_ordering.sql's own header comment.
var orderingTables = []string{
	"tables",
	"orders",
	"order_items",
	"table_sessions",
}

// TestOrderingMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy
// runs the real migration chain (via db.Migrate, exercised through
// db.TestPostgres) against an ephemeral Postgres container and then
// inspects Postgres' own catalog -- not application code -- to prove
// every table 0009_ordering.sql creates is actually row-level-security
// protected, not just declared as such in the .sql source.
func TestOrderingMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, table := range orderingTables {
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
