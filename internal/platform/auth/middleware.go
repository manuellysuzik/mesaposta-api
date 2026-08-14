// Package auth provides mesaposta-api's HTTP authentication/authorization
// middleware: Authenticate and RequireRole. This is the single seam that
// stitches together internal/platform/auth/token (verifying the access
// token), internal/platform/tenant (turning its raw tenant_id claim into a
// trusted tenant.TenantID) and internal/platform/httpx (writing a uniform
// error envelope on failure) -- and, per architecture rule R6, the only
// production package (besides _test.go files anywhere) allowed to call
// tenant.Parse: every other caller must receive a tenant.TenantID that
// already flowed through this middleware via tenant.With.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// bearerPrefix is the exact, case-sensitive scheme prefix Authenticate
// accepts on the Authorization header, per RFC 6750. Any other scheme
// (including a lowercase "bearer") is rejected.
const bearerPrefix = "Bearer "

// claimsContextKey is an unexported type used exclusively as the
// context.Context key under which the authenticated request's token.Claims
// are stored, following the same forge-proof pattern as
// internal/platform/tenant's own context key: only this package can
// construct a value of this type, so nothing outside it can inject or
// overwrite claims via context.WithValue.
type claimsContextKey struct{}

var ctxClaimsKey = claimsContextKey{}

// withClaims returns a copy of ctx carrying claims, retrievable via
// ClaimsFromContext.
func withClaims(ctx context.Context, claims token.Claims) context.Context {
	return context.WithValue(ctx, ctxClaimsKey, claims)
}

// ClaimsFromContext extracts the token.Claims Authenticate attached to ctx
// after successfully verifying the request's access token. ok is false if
// ctx never went through Authenticate.
func ClaimsFromContext(ctx context.Context) (claims token.Claims, ok bool) {
	claims, ok = ctx.Value(ctxClaimsKey).(token.Claims)
	return claims, ok
}

// ActorIDFrom extracts the best available identifier for "who is making
// this request", for callers recording an audit trail (see
// internal/platform/audit.Entry.ActorID): the authenticated access token's
// "sub" claim, as attached to ctx by Authenticate.
//
// It returns the empty string -- never an error, never a placeholder --
// when ctx carries no claims. This is deliberate on two counts. First, an
// audit trail must never be able to BLOCK the operation it observes:
// failing a legitimate mutation because the actor could not be identified
// trades a real business outcome for a bookkeeping detail. Second, an
// empty actor is persisted as SQL NULL (see
// internal/platform/audit.Entry.ActorID), so "we could not identify the
// actor" stays distinguishable from every real actor id instead of being
// papered over with something like "unknown".
//
// This is explicitly a best-effort mechanism, and the honest ceiling of
// what this system can currently know: there is no identity module, so
// "sub" is an opaque token subject, not a foreign key into any users
// table. See docs/adr/0006-audit-log.md. Contrast with a module's own,
// stricter "who is the authenticated user for THIS domain action" helper
// (e.g. internal/modules/community/infrastructure/http.currentUserID),
// which legitimately DOES fail the request on a missing/malformed
// subject -- that is a business-logic identity requirement, not an
// audit-trail best-effort one, and the two must not be conflated.
func ActorIDFrom(ctx context.Context) string {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return ""
	}
	return claims.Subject
}

// unauthenticated builds the single, uniform *errs.Error every Authenticate
// failure is reported as -- mirroring token.Verifier.Verify's own
// unauthenticated() helper: collapsing every failure mode (missing header,
// wrong scheme, empty token, invalid signature, malformed tenant_id claim,
// ...) into one Kind/Code/message denies a caller any oracle to distinguish
// *why* authentication failed.
func unauthenticated(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_token", "invalid or expired access token", cause)
}

// extractBearerToken reads and validates the shape of the Authorization
// header, returning the raw bearer token string. It never touches the
// token's contents (that's token.Verifier.Verify's job) -- only the
// transport-level "is this even a Bearer credential" question.
func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", unauthenticated(nil)
	}
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", unauthenticated(nil)
	}

	raw := strings.TrimPrefix(header, bearerPrefix)
	if raw == "" {
		return "", unauthenticated(nil)
	}
	return raw, nil
}

// Authenticate builds a middleware that:
//  1. extracts a Bearer credential from the Authorization header;
//  2. verifies it via verifier.Verify;
//  3. parses its tenant_id claim via tenant.Parse and attaches the result
//     to the request context via tenant.With;
//  4. attaches the verified token.Claims to the request context via
//     ClaimsFromContext's backing key.
//
// Any failure at any step -- including a well-signed, non-expired token
// whose tenant_id claim is not a well-formed UUID -- is reported uniformly
// as a 401 (via httpx.RespondError), never a 500: an authentication
// middleware must not turn "the caller sent something we couldn't
// validate" into an internal-server-error signal.
func Authenticate(verifier *token.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := extractBearerToken(r)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}

			claims, err := verifier.Verify(raw)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}

			tenantID, err := tenant.Parse(claims.TenantID)
			if err != nil {
				httpx.RespondError(w, r, unauthenticated(err))
				return
			}

			ctx := tenant.With(r.Context(), tenantID)
			ctx = withClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole builds a middleware that only calls the next handler if the
// authenticated caller's role (as attached to the context by Authenticate)
// is one of roles. It must run after Authenticate in the chain. Any
// mismatch -- including a request that somehow reached it without having
// gone through Authenticate at all -- is reported as errs.Forbidden (403),
// never a 500 or a 401 (the caller IS authenticated; they simply lack
// permission).
func RequireRole(roles ...token.Role) func(http.Handler) http.Handler {
	allowed := make(map[token.Role]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || !allowed[claims.Role] {
				httpx.RespondError(w, r, errs.New(errs.Forbidden, "insufficient_role",
					"you do not have permission to perform this action", nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
