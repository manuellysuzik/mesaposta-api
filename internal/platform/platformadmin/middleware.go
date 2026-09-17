// Package platformadmin authenticates requests to the platform-admin-only
// tenant provisioning API (see ./http): the surface that lets an operator
// create, list, update and suspend/reactivate tenants over HTTP, reversing
// internal/platform/db/migrations/0008_tenants.sql's original decision that
// tenant provisioning must never happen via a request mesaposta_app
// services (see docs/adr/0008-platform-tenant-provisioning.md for the
// rationale).
//
// Unlike every tenant-scoped route (internal/platform/auth.Authenticate),
// this API is NOT scoped to any tenant at all -- a platform operator
// managing every tenant on the platform has no tenant_id to put in a JWT
// claim. Authentication here is deliberately simple: a single, static
// shared secret (Config.PlatformAdminToken), compared in constant time,
// sent as the X-Platform-Admin-Token header. This is a Phase 1 decision,
// not a permanent one -- a real "platform operator" identity (its own
// users table, its own login flow) is a natural evolution once this API
// has more than one operator, left for a later phase.
package platformadmin

import (
	"crypto/subtle"
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// tokenHeader is the header a caller must present a valid platform admin
// token in. Deliberately distinct from "Authorization: Bearer" (used by
// internal/platform/auth.Authenticate's per-tenant JWTs): this is a
// different authentication mechanism entirely, and reusing the same header
// name/scheme would make it easy to confuse the two at a call site or in a
// log line.
const tokenHeader = "X-Platform-Admin-Token"

// unauthenticated builds the single, uniform *errs.Error every RequireToken
// failure is reported as -- mirroring auth.Authenticate's own
// unauthenticated() helper: collapsing "header missing" and "header present
// but wrong" into one response denies a caller any oracle to distinguish
// the two.
func unauthenticated() error {
	return errs.New(errs.Unauthenticated, "invalid_platform_admin_token", "invalid or missing platform admin token", nil)
}

// RequireToken builds a middleware that only calls the next handler if the
// request's X-Platform-Admin-Token header matches token exactly, compared
// in constant time via crypto/subtle.ConstantTimeCompare so a caller cannot
// learn anything about the real token's contents from response timing.
//
// token is expected to already be validated (non-empty, minimum length --
// see internal/platform/config.LoadFrom), so the only way this can behave
// unexpectedly is a programming error at the call site that passes an empty
// string; an empty header value is rejected outright before ever reaching
// ConstantTimeCompare, so an unconfigured (empty) token can never
// accidentally match an unset header.
func RequireToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			candidate := r.Header.Get(tokenHeader)
			if candidate == "" {
				httpx.RespondError(w, r, unauthenticated())
				return
			}

			if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
				httpx.RespondError(w, r, unauthenticated())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
