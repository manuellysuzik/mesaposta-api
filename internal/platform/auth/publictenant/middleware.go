// Package publictenant provides the HTTP middleware that establishes a
// trusted tenant.TenantID for PUBLIC, unauthenticated routes addressed by
// a tenant slug in the URL path (e.g. /public/v1/{tenantSlug}/...) --
// mirroring internal/platform/auth.Authenticate's role for JWT-bearing
// routes, but with the URL path segment as the untrusted credential
// instead of a Bearer token.
package publictenant

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// tenantSlugResolver is the narrow port this package's middleware actually
// depends on: *db.Pool satisfies it via LookupTenantBySlug. Depending on
// this interface internally -- rather than on *db.Pool everywhere below --
// lets this package's own tests exercise the full
// resolve-then-attach-to-context flow with an in-memory fake, with no
// Postgres/Docker dependency, while Middleware's exported signature still
// takes the concrete *db.Pool callers actually have in hand.
type tenantSlugResolver interface {
	LookupTenantBySlug(ctx context.Context, slug string) (rawTenantID string, err error)
}

// notFound builds the single, generic *errs.Error every failure to resolve
// urlParam into a trusted tenant.TenantID is reported as -- collapsing
// "no such slug", "slug belongs to a deactivated tenant" and "slug present
// but not even a well-formed UUID once resolved" into one indistinguishable
// response, mirroring httpx.NotFoundError's own documented pattern. cause
// carries the real reason for server-side logs only.
func notFound(cause error) error {
	return httpx.NotFoundError("tenant_not_found", "tenant not found", cause)
}

// Middleware builds an http middleware that:
//  1. reads the URL path segment named urlParam (via chi.URLParam);
//  2. resolves it to a raw tenant UUID via pool.LookupTenantBySlug;
//  3. parses that raw UUID via tenant.Parse and attaches the result to the
//     request context via tenant.With.
//
// Any failure at any step -- a missing/empty path segment, an unknown or
// inactive slug, or (defensively) a resolved value that somehow is not a
// well-formed UUID -- is reported uniformly via notFound (a 404), never a
// 500: a caller must not be able to distinguish "this table doesn't exist"
// from any other reason resolution failed.
func Middleware(pool *db.Pool, urlParam string) func(http.Handler) http.Handler {
	return resolverMiddleware(pool, urlParam)
}

// resolverMiddleware is Middleware's actual implementation, parameterized
// over the narrow tenantSlugResolver port instead of the concrete *db.Pool
// -- see tenantSlugResolver's doc comment for why.
func resolverMiddleware(resolver tenantSlugResolver, urlParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimSpace(chi.URLParam(r, urlParam))
			if slug == "" {
				httpx.RespondError(w, r, notFound(nil))
				return
			}

			rawTenantID, err := resolver.LookupTenantBySlug(r.Context(), slug)
			if err != nil {
				httpx.RespondError(w, r, notFound(err))
				return
			}

			tenantID, err := tenant.Parse(rawTenantID)
			if err != nil {
				httpx.RespondError(w, r, notFound(err))
				return
			}

			ctx := tenant.With(r.Context(), tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
