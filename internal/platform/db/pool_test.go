package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// TestOpen_InvalidDSN_ReturnsSanitizedError covers Open's DSN-parsing
// failure path without needing a real database: pgxpool.ParseConfig fails
// entirely offline for a malformed connection string.
func TestOpen_InvalidDSN_ReturnsSanitizedError(t *testing.T) {
	// An unescaped space in the userinfo component fails net/url parsing
	// inside pgxpool.ParseConfig.
	const password = "super secret password"
	dsn := "postgres://someuser:" + password + "@example.invalid:5432/db"

	pool, err := Open(context.Background(), dsn)
	if err == nil {
		t.Fatal("Open: want error for a malformed DSN, got nil")
	}
	if pool != nil {
		t.Fatal("Open: want a nil *Pool alongside a non-nil error")
	}
	if !errs.IsKind(err, errs.Internal) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Internal", errs.KindOf(err))
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("Open error = %q, must never leak the DSN's password", err.Error())
	}
	if strings.Contains(err.Error(), dsn) {
		t.Fatalf("Open error = %q, must never leak the raw DSN", err.Error())
	}
	if strings.Contains(err.Error(), "example.invalid") {
		t.Fatalf("Open error = %q, must never leak any part of the DSN (host included)", err.Error())
	}
}

// TestOpen_ValidDSN_IsLazy confirms Open never eagerly dials the database:
// pgxpool's pool construction is lazy, so a syntactically valid DSN
// pointing at a host nothing is listening on still succeeds, and the
// resulting Pool's basic lifecycle (Stat, Close, idempotent Close) works
// without ever touching the network. This is the only db_test.go coverage
// of that lifecycle that does not require -tags=integration and a real
// Postgres instance.
func TestOpen_ValidDSN_IsLazy(t *testing.T) {
	const maxConns = int32(4)
	pool, err := Open(context.Background(), "postgres://user:pass@127.0.0.1:1/db",
		WithMaxConns(maxConns))
	if err != nil {
		t.Fatalf("Open: unexpected error for a syntactically valid DSN: %v", err)
	}
	if pool == nil {
		t.Fatal("Open: want a non-nil *Pool")
	}

	if got := pool.Stat().MaxConns(); got != maxConns {
		t.Errorf("Stat().MaxConns() = %d, want %d", got, maxConns)
	}

	pool.Close()
	pool.Close() // must not panic
}

// TestPool_Scope_NoTenantInContext confirms Scope rejects a context with no
// authenticated tenant BEFORE ever touching the underlying pool: a zero
// Pool{} (nil *pgxpool.Pool) is deliberately used here, so this test would
// panic on a nil-pointer dereference if Scope tried to acquire a connection
// before checking for a tenant.
func TestPool_Scope_NoTenantInContext(t *testing.T) {
	pool := &Pool{}

	scope, err := pool.Scope(context.Background())
	if err == nil {
		t.Fatal("Scope: want error for a context with no tenant, got nil")
	}
	if scope != nil {
		t.Fatal("Scope: want a nil *Scope alongside a non-nil error")
	}
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Unauthenticated", errs.KindOf(err))
	}
}

func TestOptions_ApplyToConfig(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}

	opts := []Option{
		WithMaxConns(7),
		WithMinConns(2),
		WithMaxConnLifetime(2 * time.Hour),
		WithMaxConnIdleTime(45 * time.Minute),
		WithConnectTimeout(3 * time.Second),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.MaxConns != 7 {
		t.Errorf("MaxConns = %d, want 7", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("MinConns = %d, want 2", cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 2*time.Hour {
		t.Errorf("MaxConnLifetime = %v, want 2h", cfg.MaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != 45*time.Minute {
		t.Errorf("MaxConnIdleTime = %v, want 45m", cfg.MaxConnIdleTime)
	}
	if cfg.ConnConfig.ConnectTimeout != 3*time.Second {
		t.Errorf("ConnConfig.ConnectTimeout = %v, want 3s", cfg.ConnConfig.ConnectTimeout)
	}
}

func TestOptions_ZeroAndNegativeValuesAreIgnored(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	originalMaxConns := cfg.MaxConns
	originalMaxConnLifetime := cfg.MaxConnLifetime

	WithMaxConns(0)(cfg)
	WithMaxConns(-1)(cfg)
	WithMaxConnLifetime(0)(cfg)
	WithConnectTimeout(-5 * time.Second)(cfg)

	if cfg.MaxConns != originalMaxConns {
		t.Errorf("MaxConns changed to %d for a non-positive override, want unchanged %d", cfg.MaxConns, originalMaxConns)
	}
	if cfg.MaxConnLifetime != originalMaxConnLifetime {
		t.Errorf("MaxConnLifetime changed for a zero override, want unchanged %v", originalMaxConnLifetime)
	}
}

func TestOptions_MinConnsZeroIsRespected(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	cfg.MinConns = 5 // start from a non-zero value

	WithMinConns(0)(cfg)

	if cfg.MinConns != 0 {
		t.Errorf("MinConns = %d, want 0 (an explicit zero override must be respected)", cfg.MinConns)
	}
}
