//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file is the single most security-critical test in this task: it
// proves 0011_users.sql's two-permissive-policy design (tenant_isolation OR
// login_lookup) behaves exactly as documented in that migration's own
// header comment, against a REAL Postgres instance -- not just by reading
// the SQL and reasoning about it.

// mustTenantID parses s (a canonical UUID literal) into a tenant.TenantID,
// failing the test on any error. Duplicated (not imported) from
// internal/platform/db's own scope_integration_test.go helper of the same
// name: that one lives in package db (internal to this module), while this
// file is package migrations_test (external, black-box) -- the two
// packages cannot share unexported test helpers.
func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

// seedUser inserts a user row directly via the administrative connection
// (bypassing RLS entirely, same rationale as tenant_registry_integration_test.go's
// seedTenant): the path under test (mesaposta_app, via db.Pool) has no
// reason to ever insert data for a test fixture that ISN'T exercising
// Insert itself.
func seedUser(t *testing.T, adminConn *pgx.Conn, id, tenantID, email, passwordHash, role string, isActive bool) {
	t.Helper()
	_, err := adminConn.Exec(context.Background(), `
		INSERT INTO users (id, tenant_id, email, password_hash, role, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now(), now())
	`, id, tenantID, email, passwordHash, role, isActive)
	if err != nil {
		t.Fatalf("seeding user %q: %v", email, err)
	}
}

// --- catalog-shape checks: RLS enabled/forced, both policies exist --------

func TestUsersMigration_RLSEnabledAndForced_WithBothPolicies(t *testing.T) {
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
		WHERE relname = 'users' AND relkind = 'r'
	`).Scan(&relRowSecurity, &relForceRowSecurity)
	if err != nil {
		t.Fatalf("reading pg_class for table \"users\": %v", err)
	}
	if !relRowSecurity {
		t.Errorf("table \"users\": relrowsecurity = false, want true (ENABLE ROW LEVEL SECURITY)")
	}
	if !relForceRowSecurity {
		t.Errorf("table \"users\": relforcerowsecurity = false, want true (FORCE ROW LEVEL SECURITY)")
	}

	for _, policy := range []string{"tenant_isolation", "login_lookup"} {
		var policyCount int
		err = conn.QueryRow(ctx, `
			SELECT count(*) FROM pg_policies WHERE tablename = 'users' AND policyname = $1
		`, policy).Scan(&policyCount)
		if err != nil {
			t.Fatalf("reading pg_policies for table \"users\", policy %q: %v", policy, err)
		}
		if policyCount != 1 {
			t.Errorf("table \"users\": found %d policy(ies) named %q, want exactly 1", policyCount, policy)
		}
	}
}

// --- (a) with tenant A in ctx, a direct query does NOT see tenant B's users ---

func TestUsersRLS_TenantScopedQuery_DoesNotSeeOtherTenantsUsers(t *testing.T) {
	adminURL, appURL := db.TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	tenantA := mustTenantID(t, "aaaaaaaa-1111-1111-1111-111111111111")
	tenantB := mustTenantID(t, "bbbbbbbb-2222-2222-2222-222222222222")

	seedUser(t, adminConn, "10000000-0000-0000-0000-000000000001", tenantA.String(),
		"owner-a@example.com", dummyEncodedHash, "manager", true)
	seedUser(t, adminConn, "20000000-0000-0000-0000-000000000002", tenantB.String(),
		"owner-b@example.com", dummyEncodedHash, "manager", true)

	pool, err := db.Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctxA := tenant.With(ctx, tenantA)
	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A): %v", err)
	}
	defer scopeA.Rollback(ctxA)

	rows, err := scopeA.Query(ctxA, "SELECT email FROM users")
	if err != nil {
		t.Fatalf("selecting users as tenant A: %v", err)
	}
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			t.Fatalf("scanning email: %v", err)
		}
		emails = append(emails, email)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if len(emails) != 1 || emails[0] != "owner-a@example.com" {
		t.Fatalf("tenant A's scoped query saw %v, want exactly [owner-a@example.com] (tenant B's user must be hidden)", emails)
	}
}

// --- (b) LookupUserCredentialsByEmail (no tenant in ctx) finds a user of ANY tenant ---

func TestUsersRLS_LookupUserCredentialsByEmail_FindsUserOfAnyTenant(t *testing.T) {
	adminURL, appURL := db.TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	tenantA := mustTenantID(t, "aaaaaaaa-3333-3333-3333-333333333333")
	tenantB := mustTenantID(t, "bbbbbbbb-4444-4444-4444-444444444444")

	seedUser(t, adminConn, "30000000-0000-0000-0000-000000000003", tenantA.String(),
		"staff-a@example.com", dummyEncodedHash, "employee", true)
	seedUser(t, adminConn, "40000000-0000-0000-0000-000000000004", tenantB.String(),
		"staff-b@example.com", dummyEncodedHash, "employee", true)

	pool, err := db.Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	// Deliberately a bare context.Background(): no tenant established at
	// all, exactly like a fresh login request.
	credsA, err := pool.LookupUserCredentialsByEmail(context.Background(), "staff-a@example.com")
	if err != nil {
		t.Fatalf("LookupUserCredentialsByEmail(staff-a): unexpected error: %v", err)
	}
	if credsA.TenantID != tenantA.String() {
		t.Errorf("credsA.TenantID = %q, want %q", credsA.TenantID, tenantA.String())
	}

	credsB, err := pool.LookupUserCredentialsByEmail(context.Background(), "staff-b@example.com")
	if err != nil {
		t.Fatalf("LookupUserCredentialsByEmail(staff-b): unexpected error: %v", err)
	}
	if credsB.TenantID != tenantB.String() {
		t.Errorf("credsB.TenantID = %q, want %q", credsB.TenantID, tenantB.String())
	}
}

// --- (c) a query WITH tenant in ctx cannot use login_lookup to see another tenant's users ---

// TestUsersRLS_TenantIsolationWinsOverLoginLookup is the test that pins
// down the property 0011_users.sql's header comment calls out as the one
// way this table's two-policy design could silently break: once the
// app.tenant_id GUC IS set, login_lookup's own USING clause (`... IS
// NULL`) is false, so it contributes nothing -- an authenticated,
// tenant-scoped query can never see another tenant's row through it. This
// re-proves (a) from queries that read every column (not just email,
// unlike TestUsersRLS_TenantScopedQuery_DoesNotSeeOtherTenantsUsers above)
// AND directly targets the other tenant's known row by id, ruling out any
// possibility that login_lookup's predicate is accidentally satisfied for
// an authenticated caller.
func TestUsersRLS_TenantIsolationWinsOverLoginLookup(t *testing.T) {
	adminURL, appURL := db.TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	tenantA := mustTenantID(t, "aaaaaaaa-5555-5555-5555-555555555555")
	tenantB := mustTenantID(t, "bbbbbbbb-6666-6666-6666-666666666666")

	const otherTenantUserID = "50000000-0000-0000-0000-000000000005"
	seedUser(t, adminConn, otherTenantUserID, tenantB.String(),
		"secret-b@example.com", dummyEncodedHash, "manager", true)

	pool, err := db.Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctxA := tenant.With(ctx, tenantA)
	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A): %v", err)
	}
	defer scopeA.Rollback(ctxA)

	// Direct, targeted lookup by tenant B's known user id -- if
	// login_lookup ever won here, this would return the row.
	var count int
	if err := scopeA.QueryRow(ctxA, "SELECT count(*) FROM users WHERE id = $1", otherTenantUserID).Scan(&count); err != nil {
		t.Fatalf("counting tenant B's user as tenant A: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant A saw tenant B's user via a direct id lookup (count=%d), want 0: login_lookup must not activate once app.tenant_id is set", count)
	}

	// A bare, unfiltered SELECT * must also see nothing (tenant A seeded no
	// users of its own in this test).
	var total int
	if err := scopeA.QueryRow(ctxA, "SELECT count(*) FROM users").Scan(&total); err != nil {
		t.Fatalf("counting all visible users as tenant A: %v", err)
	}
	if total != 0 {
		t.Fatalf("tenant A's unfiltered SELECT saw %d row(s), want 0", total)
	}

	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// dummyEncodedHash is a syntactically valid (but not otherwise meaningful)
// Argon2id PHC-encoded hash string, used only to satisfy this table's
// password_hash NOT NULL column in fixtures that never actually verify a
// password.
const dummyEncodedHash = "$argon2id$v=19$m=65536,t=2,p=4$44oVFUpfPVKmA9pF40ww+Q$SgGEJok+6z9YiwLTH7wPA4EHMslah7+012DGLxkrtzk"
