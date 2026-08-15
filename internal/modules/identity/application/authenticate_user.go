package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// dummyPasswordHash is a pre-computed, syntactically valid Argon2id
// PHC-encoded hash of an arbitrary, unused string ("dummy-anti-enumeration-password"),
// generated once via internal/platform/auth/password.Hash and pasted here
// as a literal. AuthenticateUser runs password.Verify against THIS hash --
// exactly as expensive as verifying against a real user's hash, since the
// PHC string embeds its own cost parameters -- whenever the requested
// email does not resolve to a real user, so that a login attempt against a
// nonexistent email takes (as close to) the same wall-clock time as one
// against a real email with a wrong password. Without this, an attacker
// timing responses could distinguish "no such account" from "wrong
// password" purely from response latency (Argon2id's whole cost is CPU/
// memory-bound work that a nonexistent-user code path would otherwise
// simply skip), defeating the point of AuthenticateUser's uniform
// invalidCredentialsErr response below.
const dummyPasswordHash = "$argon2id$v=19$m=65536,t=2,p=4$44oVFUpfPVKmA9pF40ww+Q$SgGEJok+6z9YiwLTH7wPA4EHMslah7+012DGLxkrtzk"

// UserCredentialsLookup is the narrow port AuthenticateUser depends on for
// resolving a login attempt's email to a user's stored credentials --
// satisfied by *internal/platform/db.Pool (via its
// LookupUserCredentialsByEmail method) in production, and by a
// hand-written fake in this package's own tests (see fakes_test.go).
// AuthenticateUser deliberately depends on this narrow interface, not
// domain.UnitOfWork: see this package's own doc comment for why a login
// attempt -- which has no tenant established yet -- cannot go through the
// ordinary UnitOfWork/Repositories seam every other use case in this
// package uses.
type UserCredentialsLookup interface {
	LookupUserCredentialsByEmail(ctx context.Context, email string) (db.UserCredentials, error)
}

// AuthenticateUserInput carries a login attempt's raw email/password.
type AuthenticateUserInput struct {
	Email    string
	Password string
}

// AuthenticatedUser is what a successful login resolves to: just enough
// for the HTTP layer to issue a session (an access token plus a refresh
// token cookie). UserID and TenantID are deliberately raw UUID strings, not
// domain.ID / tenant.TenantID -- mirroring
// internal/platform/auth/refresh.Record's own "not yet validated" fields --
// because converting them into this codebase's trusted types requires
// tenant.Parse, which architecture rule R6 reserves for
// internal/platform/auth/** and, per this module's own documented
// extension of that rule, ../infrastructure/http (see architecture/rules.go's
// own comment on that extension). This package (application/) is
// deliberately NOT on that list.
type AuthenticatedUser struct {
	// UserID is the authenticated user's raw UUID text.
	UserID string
	// TenantID is the authenticated user's tenant's raw UUID text.
	TenantID string
	// Role is the authenticated user's role.
	Role domain.Role
}

// invalidCredentialsErr builds the single, uniform *errs.Error every
// AuthenticateUser failure is reported as -- whether the email does not
// resolve to any user, the password does not match, the stored role is
// somehow unparseable, or the user has been deactivated. Collapsing every
// failure mode into one Kind/Code/message denies a caller any oracle to
// distinguish "no such account" from "wrong password" from "account
// deactivated" -- see this function's own doc comment for the matching
// timing-side-channel defense (dummyPasswordHash).
func invalidCredentialsErr(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_credentials", "invalid email or password", cause)
}

// AuthenticateUser verifies a login attempt's email/password against the
// stored user (looked up directly via lookup, bypassing UnitOfWork -- see
// this package's own doc comment), returning enough to issue a session on
// success.
//
// Every failure path -- unknown email, wrong password, an unparseable
// stored role (a data-integrity bug, extremely unlikely but still handled
// uniformly rather than as a 500), and a deactivated account -- returns the
// exact same invalidCredentialsErr. password.Verify is ALWAYS called,
// against dummyPasswordHash when the email lookup itself failed, so a
// nonexistent email and a real email with a wrong password cost the same
// wall-clock time to reject (see dummyPasswordHash's own doc comment).
func AuthenticateUser(ctx context.Context, lookup UserCredentialsLookup, input AuthenticateUserInput) (AuthenticatedUser, error) {
	creds, lookupErr := lookup.LookupUserCredentialsByEmail(ctx, input.Email)

	hashToVerify := dummyPasswordHash
	if lookupErr == nil {
		hashToVerify = creds.PasswordHash
	}
	match, verifyErr := password.Verify(input.Password, hashToVerify)

	if lookupErr != nil {
		return AuthenticatedUser{}, invalidCredentialsErr(lookupErr)
	}
	if verifyErr != nil {
		return AuthenticatedUser{}, invalidCredentialsErr(verifyErr)
	}
	if !match {
		return AuthenticatedUser{}, invalidCredentialsErr(nil)
	}
	if !creds.IsActive {
		return AuthenticatedUser{}, invalidCredentialsErr(nil)
	}

	role, err := domain.ParseRole(creds.Role)
	if err != nil {
		return AuthenticatedUser{}, invalidCredentialsErr(err)
	}

	return AuthenticatedUser{UserID: creds.ID, TenantID: creds.TenantID, Role: role}, nil
}
