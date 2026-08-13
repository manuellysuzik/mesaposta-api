//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// seedTenant inserts a tenant row directly via the administrative
// connection (which, as the container's superuser, bypasses RLS
// entirely) -- LookupTenantBySlug itself runs against the restricted
// mesaposta_app role, which has no INSERT grant on tenants at all (see
// migrations/0008_tenants.sql), so tests must seed data through a
// different path than the one under test.
func seedTenant(t *testing.T, adminConn *pgx.Conn, id, slug, name string, isActive bool) {
	t.Helper()
	_, err := adminConn.Exec(context.Background(),
		"INSERT INTO tenants (id, slug, name, is_active) VALUES ($1, $2, $3, $4)",
		id, slug, name, isActive)
	if err != nil {
		t.Fatalf("seeding tenant %q: %v", slug, err)
	}
}

func TestPool_LookupTenantBySlug_ActiveTenant_ReturnsRawID(t *testing.T) {
	adminURL, appURL := TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	const wantID = "11111111-1111-1111-1111-111111111111"
	seedTenant(t, adminConn, wantID, "mesa-1", "Mesa 1", true)

	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	got, err := pool.LookupTenantBySlug(ctx, "mesa-1")
	if err != nil {
		t.Fatalf("LookupTenantBySlug: unexpected error: %v", err)
	}
	if got != wantID {
		t.Errorf("LookupTenantBySlug = %q, want %q", got, wantID)
	}
}

func TestPool_LookupTenantBySlug_InactiveTenant_ReturnsNotFound(t *testing.T) {
	adminURL, appURL := TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	seedTenant(t, adminConn, "22222222-2222-2222-2222-222222222222", "mesa-inativa", "Mesa Inativa", false)

	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	_, err = pool.LookupTenantBySlug(ctx, "mesa-inativa")
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestPool_LookupTenantBySlug_UnknownSlug_ReturnsNotFound(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	_, err = pool.LookupTenantBySlug(ctx, "does-not-exist")
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

// TestPool_LookupTenantBySlug_NoTenantInContext confirms LookupTenantBySlug
// works with a bare, tenant-free context.Context -- unlike Scope, it must
// NOT require an already-authenticated tenant, since its entire purpose is
// to establish one for the first time.
func TestPool_LookupTenantBySlug_NoTenantInContext(t *testing.T) {
	adminURL, appURL := TestPostgres(t)

	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting with the administrative DSN: %v", err)
	}
	defer adminConn.Close(ctx)

	const wantID = "33333333-3333-3333-3333-333333333333"
	seedTenant(t, adminConn, wantID, "mesa-sem-tenant-ctx", "Mesa", true)

	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	// ctx deliberately carries no tenant.TenantID (tenant.With was never
	// called) -- this must still succeed.
	got, err := pool.LookupTenantBySlug(context.Background(), "mesa-sem-tenant-ctx")
	if err != nil {
		t.Fatalf("LookupTenantBySlug: unexpected error: %v", err)
	}
	if got != wantID {
		t.Errorf("LookupTenantBySlug = %q, want %q", got, wantID)
	}
}
