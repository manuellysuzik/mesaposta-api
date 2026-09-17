// Package application implements the identity module's use cases:
// AuthenticateUser (login), CreateUser (manager invites staff),
// RefreshSession (rotate a refresh token, re-issue an access token) and
// Logout (revoke a refresh token's whole family).
//
// Unlike every other module's application layer in this codebase,
// AuthenticateUser does NOT go through domain.UnitOfWork: a login attempt
// has no tenant established yet (that is exactly what AuthenticateUser's
// job is to discover), so it reads directly from
// internal/platform/db.Pool.LookupUserCredentialsByEmail instead -- see
// that function's own doc comment, and authenticate_user.go's, for the
// full reasoning. Every other use case in this package goes through
// UnitOfWork exactly like every other module's application layer does.
package application
