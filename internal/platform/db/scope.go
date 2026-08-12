package db

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// tenantGUCSettingSQL populates the session-local "app.tenant_id" setting
// every tenant-scoped Postgres row-level-security policy in this codebase
// checks (see migrations/0001_baseline.sql). The third argument to
// set_config -- true -- is what makes this LOCAL to the current
// transaction rather than SET for the lifetime of the underlying
// connection: without it, the value would survive this Scope's Commit/
// Rollback and leak into whichever tenant's Scope next reuses the same
// pooled physical connection (see scope_integration_test.go,
// TestScope_SetLocalTenantIDDoesNotLeakAcrossPoolReuse -- the single most
// important test in this package).
const tenantGUCSettingSQL = `SELECT set_config('app.tenant_id', $1, true)`

// Scope is a single tenant-scoped Postgres transaction. It is the ONLY type
// in this package exposing Query/Exec/QueryRow: obtaining one requires an
// authenticated tenant (see Pool.Scope), and every query run through it is
// subject to the tenant_isolation row-level-security policy applied to the
// current transaction via tenantGUCSettingSQL above.
//
// Callers MUST defer a call to Rollback immediately after a successful
// Scope call, then explicitly Commit once all writes have been issued --
// mirroring the standard database/sql *Tx pattern:
//
//	scope, err := pool.Scope(ctx)
//	if err != nil {
//		return err
//	}
//	defer scope.Rollback(ctx) // no-op once Commit has succeeded
//
//	... queries against scope ...
//
//	return scope.Commit(ctx)
//
// This is what guarantees an abandoned Scope (a caller that returns early,
// panics, or simply forgets to Commit) never leaks its pooled connection:
// Rollback is idempotent and safe to call unconditionally.
type Scope struct {
	tx       pgx.Tx
	tenantID tenant.TenantID

	mu   sync.Mutex
	done bool
}

// Scope starts a new tenant-scoped transaction. It requires ctx to carry an
// authenticated tenant.TenantID (see internal/platform/tenant.From): if it
// does not, Scope returns tenant.From's *errs.Error (Kind
// errs.Unauthenticated) WITHOUT acquiring any connection from the pool --
// there is no code path in this package that touches the database on
// behalf of an unauthenticated caller.
func (p *Pool) Scope(ctx context.Context) (*Scope, error) {
	tenantID, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, errs.New(errs.Internal, "db_scope_begin_failed",
			"failed to start a tenant-scoped transaction", err)
	}

	if _, err := tx.Exec(ctx, tenantGUCSettingSQL, tenantID.String()); err != nil {
		_ = tx.Rollback(ctx)
		return nil, errs.New(errs.Internal, "db_scope_init_failed",
			"failed to initialize tenant scope", err)
	}

	return &Scope{tx: tx, tenantID: tenantID}, nil
}

// TenantID returns the tenant this Scope is bound to.
func (s *Scope) TenantID() tenant.TenantID {
	return s.tenantID
}

// Exec runs sql (with args) within this Scope's transaction.
func (s *Scope) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.tx.Exec(ctx, sql, args...)
}

// Query runs sql (with args) within this Scope's transaction and returns
// the resulting rows. Callers must Close the returned pgx.Rows.
func (s *Scope) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.tx.Query(ctx, sql, args...)
}

// QueryRow runs sql (with args) within this Scope's transaction, expecting
// at most one row.
func (s *Scope) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.tx.QueryRow(ctx, sql, args...)
}

// Commit commits the underlying transaction, releasing its connection back
// to the pool. Calling Commit more than once (or after Rollback) returns
// errScopeAlreadyDone without touching the database again.
func (s *Scope) Commit(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return errScopeAlreadyDone
	}
	s.done = true

	if err := s.tx.Commit(ctx); err != nil {
		return errs.New(errs.Internal, "db_scope_commit_failed", "failed to commit tenant scope", err)
	}
	return nil
}

// Rollback rolls back the underlying transaction, releasing its connection
// back to the pool. It is idempotent: once the Scope is already done (via
// a prior Commit or Rollback), Rollback is a safe no-op -- this is what
// lets every caller unconditionally `defer scope.Rollback(ctx)` right after
// a successful Scope call without it undoing a subsequent Commit.
func (s *Scope) Rollback(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return nil
	}
	s.done = true

	if err := s.tx.Rollback(ctx); err != nil {
		return errs.New(errs.Internal, "db_scope_rollback_failed", "failed to roll back tenant scope", err)
	}
	return nil
}

// errScopeAlreadyDone is returned by Commit when the Scope has already been
// committed or rolled back.
var errScopeAlreadyDone = errs.New(errs.Internal, "db_scope_already_done",
	"tenant scope has already been committed or rolled back", nil)
