package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/refresh"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// invalidRefreshTokenErr builds the single, uniform *errs.Error every
// refresh/logout failure this package reports is built from -- mirroring
// internal/platform/auth/refresh's own (unexported) uniform error exactly,
// duplicated here because that helper is not exported across package
// boundaries. Collapsing "token unknown to refresh.Rotate", "user not
// found", and "user deactivated" into the same Kind/Code/message denies a
// caller any oracle distinguishing those cases.
func invalidRefreshTokenErr(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_refresh_token", "invalid, expired, or revoked refresh token", cause)
}

// RefreshedSession is what a successful RefreshSession resolves to: enough
// for the HTTP layer to set a new refresh-token cookie and issue a new
// access token.
type RefreshedSession struct {
	// NewCookiePlaintext is the newly rotated refresh token's plaintext,
	// ready to be handed to internal/platform/auth/refresh.NewCookie.
	NewCookiePlaintext string
	// NewFamilyID is the (unchanged, across a rotation) refresh-token
	// family id, used as the new access token's SessionID (see
	// internal/platform/auth/token.Issuer.Issue's sessionID parameter).
	NewFamilyID string
	// UserID is the session's user, as a raw UUID string (see
	// AuthenticatedUser's own doc comment for why this package's exported
	// results stay in raw-string form rather than domain.ID/tenant.TenantID).
	UserID string
	// Role is the user's CURRENT role, re-read from the database rather
	// than trusted from any earlier token -- see RefreshSession's own doc
	// comment for why this matters.
	Role domain.Role
}

// RefreshSession rotates the presented refresh token (via refresh.Rotate,
// which itself detects and punishes token reuse -- see that function's own
// doc comment) and re-resolves the session's user via uow, WITHIN the
// tenant already established in ctx (the caller -- ../infrastructure/http's
// refresh handler -- is responsible for having called tenant.With before
// invoking this function; see that handler's own doc comment for the
// "X-Tenant-ID hint" design this requires).
//
// Re-reading the user (rather than trusting the ROLE embedded in whatever
// access token accompanied the original login) is deliberate: it is what
// makes a role change or account deactivation that happened AFTER login
// take effect the next time the session refreshes, instead of only at the
// access token's next natural 15-minute expiry combined with a fresh
// login. A deactivated user's refresh is rejected with the exact same
// invalidRefreshTokenErr a genuinely invalid/expired/reused token would
// produce -- never a distinguishable error -- even though, by this point,
// refresh.Rotate has already committed the rotation (the old token is
// marked rotated; a new, unused Record now exists in the family). This is
// an accepted trade-off, not an oversight: the caller never learns the new
// cookie's plaintext (RefreshSession returns before constructing
// RefreshedSession), so it can never reach the client, and the OLD cookie
// the client still holds is now rotated -- presenting it again is reported,
// correctly, as reuse, which revokes the whole family. A deactivated
// user's session therefore still ends up fully locked out, just one
// (harmless, silently wasted) Record later than the simplest possible
// design.
func RefreshSession(ctx context.Context, store refresh.RefreshStore, uow domain.UnitOfWork, presentedPlaintext string) (RefreshedSession, error) {
	newPlaintext, newRecord, err := refresh.Rotate(ctx, store, presentedPlaintext)
	if err != nil {
		return RefreshedSession{}, err
	}

	userID, err := domain.ParseID(newRecord.SubjectID)
	if err != nil {
		return RefreshedSession{}, invalidRefreshTokenErr(err)
	}

	var user *domain.User
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Users().FindByID(ctx, userID)
		if err != nil {
			return err
		}
		user = found
		return nil
	})
	if err != nil {
		return RefreshedSession{}, invalidRefreshTokenErr(err)
	}

	if !user.IsActive() {
		return RefreshedSession{}, invalidRefreshTokenErr(nil)
	}

	return RefreshedSession{
		NewCookiePlaintext: newPlaintext,
		NewFamilyID:        newRecord.FamilyID,
		UserID:             user.ID().String(),
		Role:               user.Role(),
	}, nil
}
