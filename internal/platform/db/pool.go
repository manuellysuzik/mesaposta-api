// Package db provides mesaposta-api's Postgres access layer, built around a
// single, deliberate constraint: nothing outside this package can run a SQL
// query without first proving -- at compile time, not just by convention --
// that it has an authenticated tenant in hand.
//
// Pool, the type returned by Open, exposes no Query/Exec/QueryRow methods
// and never hands out the underlying *pgxpool.Pool. Its only way to reach
// the database is Scope(ctx), which fails immediately (before acquiring any
// connection) unless ctx carries a tenant.TenantID, and returns a *Scope
// (see scope.go) that both sets Postgres row-level-security's session-local
// tenant GUC for its transaction's lifetime and is the only type in this
// package exposing Query/Exec/QueryRow. See
// architecture/testdata/fixtures/queryless_pool and
// architecture/compilefail_test.go for the automated proof that
// `pool.Query(...)` does not compile.
package db

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Option configures the *pgxpool.Config Open builds its pool from. See
// WithMaxConns, WithMinConns, WithMaxConnLifetime, WithMaxConnIdleTime and
// WithConnectTimeout.
type Option func(*pgxpool.Config)

// WithMaxConns caps the pool at n simultaneous connections.
func WithMaxConns(n int32) Option {
	return func(cfg *pgxpool.Config) {
		if n > 0 {
			cfg.MaxConns = n
		}
	}
}

// WithMinConns keeps n idle connections warm.
func WithMinConns(n int32) Option {
	return func(cfg *pgxpool.Config) {
		if n >= 0 {
			cfg.MinConns = n
		}
	}
}

// WithMaxConnLifetime recycles a pooled connection after d.
func WithMaxConnLifetime(d time.Duration) Option {
	return func(cfg *pgxpool.Config) {
		if d > 0 {
			cfg.MaxConnLifetime = d
		}
	}
}

// WithMaxConnIdleTime closes a pooled connection after it has sat idle for
// d.
func WithMaxConnIdleTime(d time.Duration) Option {
	return func(cfg *pgxpool.Config) {
		if d > 0 {
			cfg.MaxConnIdleTime = d
		}
	}
}

// WithConnectTimeout bounds how long establishing a single new connection
// may take.
func WithConnectTimeout(d time.Duration) Option {
	return func(cfg *pgxpool.Config) {
		if d > 0 && cfg.ConnConfig != nil {
			cfg.ConnConfig.ConnectTimeout = d
		}
	}
}

// Pool is a tenant-scoping handle onto a Postgres connection pool. The zero
// value is not usable; construct one via Open.
//
// Pool intentionally exposes no way to run a query without going through
// Scope: no Query/Exec/QueryRow methods, and no accessor for the wrapped
// *pgxpool.Pool from outside this package (unsafeAdmin exists solely for
// migrate.go, in this same package, to hand the raw pool to the goose
// migration runner).
type Pool struct {
	pool *pgxpool.Pool

	closeOnce sync.Once
}

// Open builds a Pool backed by databaseURL, applying opts on top of pgx's
// own parsed defaults. It does not eagerly verify connectivity -- call
// Ping, e.g. from a readiness probe, for that.
//
// Any failure here (a malformed DSN, or the pool failing to initialize) is
// reported WITHOUT the underlying pgx/pgconn error's message: that message
// can (and, for a malformed DSN, typically does) echo the DSN back verbatim
// -- including any embedded password -- so it must never reach a log line
// or an error chain a caller might print.
func Open(ctx context.Context, databaseURL string, opts ...Option) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errs.New(errs.Internal, "db_invalid_dsn",
			"invalid database connection configuration", nil)
	}

	for _, opt := range opts {
		opt(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errs.New(errs.Internal, "db_pool_init_failed",
			"failed to initialize database connection pool", nil)
	}

	return &Pool{pool: pool}, nil
}

// Ping verifies connectivity by acquiring and immediately releasing a
// connection. It honors ctx's deadline/cancellation, returning quickly
// (without ever blocking indefinitely) when ctx is already done.
func (p *Pool) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return errs.New(errs.Internal, "db_unreachable", "database is unreachable", err)
	}
	return nil
}

// Close shuts down the pool, waiting for in-flight connections to be
// released. It is safe to call more than once: only the first call has any
// effect.
func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		p.pool.Close()
	})
}

// Stat returns the pool's current connection statistics (acquired, idle,
// max, ...), primarily so an HTTP readiness probe can report on it. It
// grants no query access whatsoever.
func (p *Pool) Stat() *pgxpool.Stat {
	return p.pool.Stat()
}

// unsafeAdmin returns the raw, unrestricted *pgxpool.Pool. It exists
// exclusively for migrate.go (same package) to hand off to the goose
// migration runner via pgx's database/sql compatibility shim -- schema
// migrations legitimately need arbitrary DDL/DML access that no tenant
// Scope could ever grant. No other file, in this package or any other, may
// call this method for anything else.
func (p *Pool) unsafeAdmin() *pgxpool.Pool {
	return p.pool
}
