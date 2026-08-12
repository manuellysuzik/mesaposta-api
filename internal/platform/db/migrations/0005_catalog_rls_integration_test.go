//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// catalogTables lists every table 0005_catalog.sql introduces -- this test
// walks this exact list, confirming each one has both RLS enabled AND
// forced (relrowsecurity/relforcerowsecurity in pg_class) and a
// "tenant_isolation" policy (pg_policies), the same way 0001/0002/0003/
// 0004/0006's tables already do (see 0004_operations_rls_integration_test.go
// for the reference pattern this test mirrors for the catalog module).
var catalogTables = []string{
	"products",
	"product_ingredients",
	"recipes",
	"recipe_ingredients",
}

// TestCatalogMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy runs
// the real migration chain (via db.Migrate, exercised through
// db.TestPostgres) against an ephemeral Postgres container and then
// inspects Postgres' own catalog -- not application code -- to prove every
// table 0005_catalog.sql creates is actually row-level-security protected,
// not just declared as such in the .sql source.
func TestCatalogMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, table := range catalogTables {
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
