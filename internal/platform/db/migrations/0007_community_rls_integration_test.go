//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// communityTables lists every table 0007_community.sql introduces -- this
// test walks this exact list, confirming each one has both RLS enabled AND
// forced (relrowsecurity/relforcerowsecurity in pg_class) and a
// "tenant_isolation" policy (pg_policies), the same way 0001-0006's tables
// already do (see 0004_operations_rls_integration_test.go and
// 0006_crm_rls_integration_test.go for the identical pattern this test
// mirrors for the community module).
var communityTables = []string{
	"community_posts",
	"community_comments",
	"community_likes",
}

// TestCommunityMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy
// runs the real migration chain (via db.Migrate, exercised through
// db.TestPostgres) against an ephemeral Postgres container and then
// inspects Postgres' own catalog -- not application code -- to prove every
// table 0007_community.sql creates is actually row-level-security
// protected, not just declared as such in the .sql source.
func TestCommunityMigration_EveryTable_HasForcedRLSAndTenantIsolationPolicy(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	for _, table := range communityTables {
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

// TestCommunityLikesTable_HasUniqueConstraintOnTenantPostUser proves the
// UNIQUE (tenant_id, post_id, user_id) constraint
// internal/modules/community/infrastructure/postgres/like_repository.go's
// idempotent "INSERT ... ON CONFLICT DO NOTHING" relies on actually exists
// in the real schema, not just in this migration's own source comments.
func TestCommunityLikesTable_HasUniqueConstraintOnTenantPostUser(t *testing.T) {
	adminURL, _ := db.TestPostgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer conn.Close(ctx)

	var constraintType string
	err = conn.QueryRow(ctx, `
		SELECT contype
		FROM pg_constraint
		WHERE conname = 'community_likes_tenant_post_user_key'
	`).Scan(&constraintType)
	if err != nil {
		t.Fatalf("reading pg_constraint for community_likes_tenant_post_user_key: %v", err)
	}
	if constraintType != "u" {
		t.Errorf("community_likes_tenant_post_user_key contype = %q, want %q (UNIQUE)", constraintType, "u")
	}
}
