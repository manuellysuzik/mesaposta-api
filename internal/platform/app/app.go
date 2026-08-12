// Package app is mesaposta-api's HTTP composition root: NewApp wires the
// platform building blocks completed in Blocos A-E (internal/platform/httpx's
// router/server plumbing and health/readiness probes, internal/platform/auth's
// Authenticate/RequireRole middleware, internal/platform/auth/token's JWT
// issuer/verifier, and a tenant-scoped internal/platform/db.Pool) into the
// single http.Handler cmd/api/main.go serves.
//
// NewApp registers exactly one route beyond health/readiness: a small
// "probe" API (see probe.go) that exists solely to prove, here in Phase 0,
// that JWT authentication -> tenant context -> role authorization ->
// Postgres row-level security are correctly wired together end-to-end. It
// carries no business meaning and is not a real domain-module API -- Phase 1
// onward introduces internal/modules/** (see internal/modules/README.md)
// with real routes, and this probe route is expected to be deleted once a
// real authenticated route exists to prove the same thing.
package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"

	catalighttp "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/http"
	catalogpg "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/postgres"
	communityhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/http"
	communitypg "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/postgres"
	crmhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/http"
	crmpg "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/postgres"
	operationshttp "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/http"
	operationspg "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
)

const (
	// TokenKID identifies the single HMAC signing key mesaposta-api's
	// access tokens use in Phase 0, derived from config.Config.JWTSecret.
	// See BuildVerifier / BuildIssuer -- both must agree on this exact kid
	// (along with TokenIssuer/TokenAudience) for a token either one
	// produces/accepts to be usable by the other.
	TokenKID = "primary"

	// TokenIssuer and TokenAudience are the fixed "iss"/"aud" claims every
	// access token this service issues and verifies must carry.
	TokenIssuer   = "mesaposta-api"
	TokenAudience = "mesaposta-api"
)

// BuildVerifier builds the token.Verifier NewApp's Authenticate middleware
// uses, from cfg.JWTSecret. cfg is expected to already be validated (see
// internal/platform/config.Load), so the only way this can fail is a
// programming error in this package's own constants.
func BuildVerifier(cfg config.Config) (*token.Verifier, error) {
	return token.NewVerifier(map[string][]byte{TokenKID: []byte(cfg.JWTSecret)}, TokenIssuer, TokenAudience)
}

// BuildIssuer builds a token.Issuer matching BuildVerifier's exact
// kid/issuer/audience. Phase 0 ships no login endpoint yet -- nothing in
// cmd/api/main.go calls this today -- but it exists so both this package's
// own tests and internal/platform/httpx's end-to-end test can mint tokens
// NewApp's Authenticate middleware accepts, and so a real login handler
// introduced in a later phase has a ready-made, consistent place to build
// its Issuer from.
func BuildIssuer(cfg config.Config) (*token.Issuer, error) {
	return token.NewIssuer(TokenKID, []byte(cfg.JWTSecret), TokenIssuer, TokenAudience)
}

// NewApp builds the complete HTTP handler mesaposta-api serves: the shared
// router/middleware chain plus the unauthenticated /healthz and /readyz
// probes (internal/platform/httpx.NewRouter), and the role-protected probe
// API under /v1/_probe (see probe.go), wired through verifier and pool.
//
// logger defaults to slog.Default() when nil. pool may be nil ONLY for
// tests that never exercise a route touching the database (e.g. /healthz,
// or a request rejected by Authenticate/RequireRole before it ever reaches
// a handler) -- see app_test.go; a nil pool also makes /readyz always
// report ready, which is only ever correct in that same test scenario.
func NewApp(cfg config.Config, pool *db.Pool, logger *slog.Logger, verifier *token.Verifier) http.Handler {
	_ = cfg // reserved for future per-environment wiring; validated by the caller (config.Load).

	if logger == nil {
		logger = slog.Default()
	}

	router := httpx.NewRouter(logger, readinessCheck(pool), httpx.DefaultRequestTimeout)

	router.Route("/v1/_probe", func(r chi.Router) {
		r.Use(auth.Authenticate(verifier))
		r.With(auth.RequireRole(token.RoleGestor)).Post("/events", createProbeEventHandler(pool))
		r.With(auth.RequireRole(token.RoleGestor, token.RoleFuncionario)).Get("/events/{aggregateID}", getProbeEventHandler(pool))
	})

	router.Route("/v1", func(r chi.Router) {
		r.Use(auth.Authenticate(verifier))
		if pool != nil {
			operationshttp.Mount(r, operationspg.NewUnitOfWork(pool))
			catalighttp.Mount(r, catalogpg.NewUnitOfWork(pool))
			crmhttp.Mount(r, crmpg.NewUnitOfWork(pool))
			communityhttp.Mount(r, communitypg.NewUnitOfWork(pool))
		}
	})

	return router
}

// readinessCheck builds the httpx.ReadinessCheck NewApp's /readyz endpoint
// uses: it pings pool, honoring ctx's deadline/cancellation. A nil pool
// always reports ready -- see NewApp's doc comment for why that is only
// ever correct in tests.
func readinessCheck(pool *db.Pool) httpx.ReadinessCheck {
	return func(ctx context.Context) error {
		if pool == nil {
			return nil
		}
		return pool.Ping(ctx)
	}
}
