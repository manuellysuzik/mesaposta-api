package application

import (
	"context"
	"crypto/sha256"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/refresh"
)

// Logout revokes the ENTIRE refresh-token family the presented cookie
// belongs to (via refresh.RevokeFamily), so every refresh token ever issued
// within that login session -- not just the one presented -- stops working.
//
// Like RefreshSession, this runs WITHIN the tenant already established in
// ctx by the caller (../infrastructure/http's logout handler) -- see that
// handler's own doc comment for the "X-Tenant-ID hint" design this
// requires, and refresh.PostgresStore's own doc comment for why FindByHash
// specifically needs that tenant pre-established.
//
// presentedPlaintext not resolving to any known token is reported via the
// exact same invalidRefreshTokenErr RefreshSession uses -- an unknown/
// already-logged-out token and a successful logout are handled identically
// from the caller's perspective (the HTTP handler clears the cookie
// either way; see that handler's own doc comment).
func Logout(ctx context.Context, store refresh.RefreshStore, presentedPlaintext string) error {
	hash := sha256.Sum256([]byte(presentedPlaintext))

	record, err := store.FindByHash(ctx, hash)
	if err != nil {
		return invalidRefreshTokenErr(err)
	}

	return refresh.RevokeFamily(ctx, store, record.FamilyID)
}
