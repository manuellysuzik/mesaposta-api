//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// mustTenantID parses s (a canonical UUID literal) into a tenant.TenantID,
// failing the test on any error.
func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

func TestPool_Scope_RLS_TenantIsolation(t *testing.T) {
	_, appURL := TestPostgres(t)
	pool, err := Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	tenantA := mustTenantID(t, "11111111-1111-1111-1111-111111111111")
	tenantB := mustTenantID(t, "22222222-2222-2222-2222-222222222222")

	// Tenant A inserts two rows.
	ctxA := tenant.With(context.Background(), tenantA)
	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A): %v", err)
	}
	if _, err := scopeA.Exec(ctxA, "INSERT INTO scope_probe (tenant_id, label) VALUES ($1, 'a1'), ($1, 'a2')", tenantA.String()); err != nil {
		t.Fatalf("insert as tenant A: %v", err)
	}
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (tenant A insert): %v", err)
	}

	// Tenant B runs a bare SELECT * with no WHERE clause at all and must
	// see zero rows: RLS, not application-level filtering, is what hides
	// tenant A's rows.
	ctxB := tenant.With(context.Background(), tenantB)
	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (tenant B): %v", err)
	}
	defer scopeB.Rollback(ctxB)

	rows, err := scopeB.Query(ctxB, "SELECT id, tenant_id, label FROM scope_probe")
	if err != nil {
		t.Fatalf("select as tenant B: %v", err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw %d row(s) via an unfiltered SELECT *, want 0 (RLS must hide tenant A's rows)", count)
	}

	// Cross-tenant UPDATE/DELETE must affect zero rows too.
	tag, err := scopeB.Exec(ctxB, "UPDATE scope_probe SET label = 'tampered'")
	if err != nil {
		t.Fatalf("update as tenant B: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("tenant B's unfiltered UPDATE affected %d row(s), want 0", tag.RowsAffected())
	}

	tag, err = scopeB.Exec(ctxB, "DELETE FROM scope_probe")
	if err != nil {
		t.Fatalf("delete as tenant B: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("tenant B's unfiltered DELETE affected %d row(s), want 0", tag.RowsAffected())
	}

	if err := scopeB.Commit(ctxB); err != nil {
		t.Fatalf("commit (tenant B no-op writes): %v", err)
	}

	// Tenant A's own rows must still be exactly as inserted.
	scopeA2, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A, re-check): %v", err)
	}
	defer scopeA2.Rollback(ctxA)

	var count2 int
	if err := scopeA2.QueryRow(ctxA, "SELECT count(*) FROM scope_probe").Scan(&count2); err != nil {
		t.Fatalf("counting tenant A's own rows: %v", err)
	}
	if count2 != 2 {
		t.Fatalf("tenant A sees %d row(s) after tenant B's cross-tenant writes, want 2 (untouched)", count2)
	}
	_ = scopeA2.Commit(ctxA)
}

func TestPool_Scope_Commit_ReleasesConnection(t *testing.T) {
	_, appURL := TestPostgres(t)
	pool, err := Open(context.Background(), appURL, WithMaxConns(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	tenantA := mustTenantID(t, "33333333-3333-3333-3333-333333333333")
	ctx := tenant.With(context.Background(), tenantA)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if err := scope.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("AcquiredConns() = %d after Commit, want 0 (connection must return to the pool)", got)
	}
}

func TestPool_Scope_Rollback_ReleasesConnection(t *testing.T) {
	_, appURL := TestPostgres(t)
	pool, err := Open(context.Background(), appURL, WithMaxConns(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	tenantA := mustTenantID(t, "44444444-4444-4444-4444-444444444444")
	ctx := tenant.With(context.Background(), tenantA)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if err := scope.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("AcquiredConns() = %d after Rollback, want 0 (connection must return to the pool)", got)
	}
}

// TestPool_Scope_AbandonedScope_DeferredRollbackReleasesConnection proves
// the documented "defer scope.Rollback(ctx) right after Scope succeeds"
// pattern is exactly what prevents a connection leak when a caller never
// explicitly commits (e.g. an early return on a validation failure, or a
// panic recovered further up the stack).
func TestPool_Scope_AbandonedScope_DeferredRollbackReleasesConnection(t *testing.T) {
	_, appURL := TestPostgres(t)
	pool, err := Open(context.Background(), appURL, WithMaxConns(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	tenantA := mustTenantID(t, "55555555-5555-5555-5555-555555555555")
	ctx := tenant.With(context.Background(), tenantA)

	func() {
		scope, err := pool.Scope(ctx)
		if err != nil {
			t.Fatalf("Scope: %v", err)
		}
		defer scope.Rollback(ctx)
		// Deliberately abandoned: no Commit call, simulating a caller that
		// returned early without finishing its unit of work.
	}()

	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("AcquiredConns() = %d after an abandoned Scope's deferred Rollback, want 0 (no connection leak)", got)
	}
}

// TestScope_SetLocalTenantIDDoesNotLeakAcrossPoolReuse is the single most
// important test in this package (see internal/platform/db/scope.go's
// tenantGUCSettingSQL doc comment and migrations/0001_baseline.sql): it
// proves `set_config('app.tenant_id', $1, true)` is genuinely
// transaction-LOCAL, not session-wide, by forcing a single physical
// connection (MaxConns=1) to be reused across two different tenants'
// Scopes and directly inspecting the Postgres session setting via the raw
// admin pool -- bypassing db.Scope entirely for the inspection step, since
// Scope itself always (re)sets app.tenant_id and could otherwise mask a
// regression.
//
// If SET LOCAL regressed to a plain SET (session-scoped), this test would
// fail: the raw, Scope-free transaction opened in step 3 would still see
// tenant A's id, because MaxConns=1 guarantees it reuses the exact same
// physical connection tenant A's Scope used and committed in step 1.
func TestScope_SetLocalTenantIDDoesNotLeakAcrossPoolReuse(t *testing.T) {
	_, appURL := TestPostgres(t)
	pool, err := Open(context.Background(), appURL, WithMaxConns(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	tenantA := mustTenantID(t, "66666666-6666-6666-6666-666666666666")

	// 1. Tenant A's Scope sets app.tenant_id and commits, releasing the
	//    pool's single physical connection back to the pool.
	ctxA := tenant.With(ctx, tenantA)
	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A): %v", err)
	}
	var seenDuringScope string
	if err := scopeA.QueryRow(ctxA, "SELECT current_setting('app.tenant_id', true)").Scan(&seenDuringScope); err != nil {
		t.Fatalf("reading app.tenant_id during tenant A's own scope: %v", err)
	}
	if seenDuringScope != tenantA.String() {
		t.Fatalf("app.tenant_id during tenant A's scope = %q, want %q", seenDuringScope, tenantA.String())
	}
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (tenant A): %v", err)
	}

	// 2. With MaxConns=1, the pool has exactly one physical connection,
	//    now idle again after step 1's commit.
	if got := pool.Stat().TotalConns(); got != 1 {
		t.Fatalf("Stat().TotalConns() = %d, want exactly 1 (test requires a single physical connection)", got)
	}

	// 3. Open a RAW transaction directly against the pool's underlying
	//    *pgxpool.Pool -- deliberately bypassing db.Scope, which would
	//    otherwise always re-set app.tenant_id itself and hide a
	//    regression. This must reuse the very same physical connection
	//    tenant A's Scope just released, since MaxConns=1 leaves no other
	//    connection available.
	rawTx, err := pool.unsafeAdmin().Begin(ctx)
	if err != nil {
		t.Fatalf("beginning raw transaction: %v", err)
	}
	defer rawTx.Rollback(ctx)

	var leaked string
	err = rawTx.QueryRow(ctx, "SELECT current_setting('app.tenant_id', true)").Scan(&leaked)
	if err != nil {
		t.Fatalf("reading app.tenant_id in a fresh, Scope-free transaction: %v", err)
	}

	if leaked == tenantA.String() {
		t.Fatalf("app.tenant_id leaked across pooled-connection reuse: got %q (tenant A's id) in a brand-new "+
			"transaction that never called set_config itself -- SET LOCAL is not actually LOCAL", leaked)
	}
	if leaked != "" {
		t.Fatalf("app.tenant_id = %q in a fresh transaction that never set it, want empty string "+
			"(current_setting(..., true) on an unset GUC)", leaked)
	}
}
