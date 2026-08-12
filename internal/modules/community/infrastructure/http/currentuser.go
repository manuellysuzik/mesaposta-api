package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// currentUserID resolves the authenticated caller's own identity for the
// use cases that need it (commenting, liking, unliking) -- as opposed to
// just their tenant, which internal/platform/tenant.From alone already
// provides for every other module's read/create handlers.
//
// DESIGN DECISION (not fully specified by this module's own task notes,
// documented here since a future Users/accounts module may want to revisit
// it): mesaposta-api has NO dedicated Users/accounts module yet as of this
// phase -- internal/platform/auth/token.Claims carries only a bare "sub"
// (Subject) claim, with no accompanying users table or FK any module could
// join against (see internal/platform/auth/token/claims.go and
// internal/platform/auth/token/token.go's own Issue, which accepts any
// non-empty string as subject; its own tests use plain strings like
// "user-1", not necessarily UUIDs). Given that, currentUserID makes the
// simplest choice consistent with the rest of this codebase's identity
// conventions: it requires the "sub" claim to already BE a well-formed
// UUID (validated via this module's own domain.ParseID, exactly the same
// UUID shape every other ID in this module already uses) and treats it as
// the community module's own domain.ID for that user. A "sub" claim that
// is present but not UUID-shaped is treated as an authentication failure
// (errs.Unauthenticated, mirroring internal/platform/auth.Authenticate's
// own uniform "invalid or expired access token" treatment of a malformed
// tenant_id claim) rather than a 400/422 -- the caller's TOKEN is what is
// malformed, not their request body. When a real Users module exists, its
// issued tokens' "sub" claims are expected to already be that module's own
// UUID-shaped user id, making this validation a no-op in practice; nothing
// here needs to change.
func currentUserID(r *http.Request) (domain.ID, error) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return domain.ID{}, errs.New(errs.Unauthenticated, "missing_claims",
			"request has no authenticated user", nil)
	}

	userID, err := domain.ParseID(claims.Subject)
	if err != nil {
		return domain.ID{}, errs.New(errs.Unauthenticated, "invalid_user_id",
			"authenticated user id is not a valid identifier", err)
	}
	return userID, nil
}
