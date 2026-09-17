package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// UserCredentials is the raw row LookupUserCredentialsByEmail returns: just
// enough to authenticate a login attempt and re-establish the caller's
// tenant/role. Every field is a raw string/bool -- not a domain.ID, not a
// tenant.TenantID, not a domain.Role -- for the exact same reason
// internal/platform/auth/refresh.Record's own fields are: this type is
// produced BEFORE any tenant is established in the request, by a query that
// deliberately runs outside internal/platform/db.Pool.Scope (see this
// file's package doc comment below), so it has no business constructing any
// of this codebase's "already validated" types. Converting these raw values
// into their trusted counterparts (tenant.Parse, domain.ParseID,
// domain.ParseRole) is the caller's job, once a tenant scope actually
// exists.
type UserCredentials struct {
	// ID is the user's raw UUID text.
	ID string
	// TenantID is the raw UUID text of the tenant this user belongs to.
	TenantID string
	// Email is the user's login email, exactly as stored.
	Email string
	// PasswordHash is the Argon2id PHC-encoded hash to verify a login
	// attempt's plaintext password against (see
	// internal/platform/auth/password.Verify).
	PasswordHash string
	// Role is the user's raw role string ("manager" or "employee").
	Role string
	// IsActive reports whether this user is currently allowed to
	// authenticate. A deactivated user's credentials are still returned
	// (not hidden as "not found") so the caller can apply the exact same
	// anti-enumeration handling to "wrong password" and "deactivated
	// account" -- both collapse to the same generic invalid_credentials
	// response one layer up (see
	// internal/modules/identity/application.AuthenticateUser).
	IsActive bool
}

// lookupUserCredentialsByEmailSQL resolves a user's login row by their
// globally-unique email (see internal/platform/db/migrations/0011_users.sql
// for why email is UNIQUE across the whole users table, not just per
// tenant: a login form has no tenant context yet -- the email IS how the
// tenant gets resolved).
const lookupUserCredentialsByEmailSQL = `
	SELECT id::text, tenant_id::text, email, password_hash, role, is_active
	FROM users
	WHERE email = $1
`

// LookupUserCredentialsByEmail resolves a user's login credentials by their
// globally-unique email, without requiring a tenant already established in
// ctx.
//
// This is the SECOND and LAST deliberate exception to this package's core
// invariant that no query ever runs outside a tenant-scoped Pool.Scope --
// see LookupTenantBySlug's own doc comment (tenant_registry.go) for the
// first, and identical, exception, and 0011_users.sql's header comment for
// the matching RLS design that makes this safe: the users table's
// "login_lookup" policy is a permissive SELECT that only ever matches when
// the app.tenant_id GUC is NOT set (i.e. exactly this code path, which
// never opens a Scope), while every OTHER query against users -- every one
// that runs through Scope, with a tenant already established -- is governed
// by the ordinary "tenant_isolation" policy instead. Do not add a third
// exception without an equivalently documented, equivalently narrow
// justification: every additional query that bypasses Scope is one more
// place this codebase's single central tenant-isolation guarantee has to be
// re-proven by hand instead of enforced structurally.
//
// A missing email is reported as an *errs.Error of Kind errs.NotFound.
// Callers (specifically, the login handler) MUST NOT let that Kind -- or
// any distinction between "no such email" and "wrong password" or "account
// deactivated" -- reach the client: every login failure collapses to the
// same generic errs.Unauthenticated response, precisely to deny an attacker
// an email-enumeration oracle.
func (p *Pool) LookupUserCredentialsByEmail(ctx context.Context, email string) (UserCredentials, error) {
	var creds UserCredentials
	err := p.pool.QueryRow(ctx, lookupUserCredentialsByEmailSQL, email).Scan(
		&creds.ID, &creds.TenantID, &creds.Email, &creds.PasswordHash, &creds.Role, &creds.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserCredentials{}, errs.New(errs.NotFound, "user_not_found", "user not found", err)
		}
		return UserCredentials{}, errs.New(errs.Internal, "user_lookup_failed", "failed to resolve user by email", err)
	}
	return creds, nil
}
