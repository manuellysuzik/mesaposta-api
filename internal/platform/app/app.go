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
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/publictenant"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/platformadmin"
	platformadminhttp "github.com/manuellysuzik/mesaposta-api/internal/platform/platformadmin/http"

	catalighttp "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/http"
	catalogpg "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/postgres"
	catalogpricing "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/pricing"
	communityhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/http"
	communitypg "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/postgres"
	crmhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/http"
	crmpg "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/postgres"
	identityhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/identity/infrastructure/http"
	operationshttp "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/http"
	operationspg "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	orderinghttp "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/http"
	orderinghttppublic "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/httppublic"
	orderingpg "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/postgres"
)

const (
	// publicRateLimitMaxBuckets caps the number of distinct (client IP,
	// tenant slug) keys internal/modules/ordering's public routes track
	// simultaneously -- see internal/platform/httpx.NewRateLimiter's own
	// doc comment for why this cap exists at all (protecting against
	// unbounded memory growth from an attacker cycling through spoofed
	// keys).
	publicRateLimitMaxBuckets = 10_000
	// publicRateLimitRefillPerSecond and publicRateLimitBurst are
	// deliberately generous for a digital menu a real diner might refresh
	// a few times while deciding what to order, while still bounding an
	// automated scraper/abuse pattern hammering a single IP+tenant pair.
	// Chosen as a simple, reasonable default for this phase -- not backed
	// by production traffic data yet -- and easy to retune later without
	// any call-site change beyond these two constants.
	publicRateLimitRefillPerSecond = 2
	publicRateLimitBurst           = 20
)

const (
	// authRateLimitMaxBuckets caps the number of distinct client IPs
	// /v1/auth tracks simultaneously -- same rationale as
	// publicRateLimitMaxBuckets above.
	authRateLimitMaxBuckets = 10_000
	// authRateLimitRefillPerSecond and authRateLimitBurst bound login
	// attempts to 10 per minute per client IP. This exists to blunt
	// automated credential-stuffing against POST /v1/auth/login -- the
	// timing-attack defense in application.AuthenticateUser (see its own
	// doc comment) only equalizes response latency, it does nothing to cap
	// attempt volume. 10/min is deliberately not as tight as a typical
	// single-user login throttle: this key is the raw client IP, and a
	// real customer is a restaurant, where several staff behind the same
	// NAT'd IP may legitimately log in within seconds of each other (e.g.
	// at shift start) -- a stricter per-IP limit would lock out real
	// employees before it meaningfully slowed a distributed attacker.
	// Chosen as a simple, reasonable default for this phase -- not backed
	// by production traffic data yet -- and easy to retune later without
	// any call-site change beyond these two constants.
	authRateLimitRefillPerSecond = 10.0 / 60.0
	authRateLimitBurst           = 10
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
// probes (internal/platform/httpx.NewRouter), the role-protected probe API
// under /v1/_probe (see probe.go), every domain module's authenticated
// routes under /v1, and internal/modules/ordering's PUBLIC digital-menu
// routes under /public/v1/{tenantSlug} -- wired through verifier and pool.
//
// metricsHandler, when non-nil, is mounted at GET /metrics by
// httpx.NewRouter -- see that function's own doc comment. Pass nil to
// disable the route entirely (e.g. observability.Setup returned a disabled
// Handle, or a test that does not need it).
//
// logger defaults to slog.Default() when nil. pool may be nil ONLY for
// tests that never exercise a route touching the database (e.g. /healthz,
// or a request rejected by Authenticate/RequireRole before it ever reaches
// a handler) -- see app_test.go; a nil pool also makes /readyz always
// report ready, which is only ever correct in that same test scenario.
//
// LIFECYCLE NOTE on the *httpx.RateLimiter built below for
// /public/v1/{tenantSlug} (and the second one mountAuthRoutes builds for
// /v1/auth): NewApp has no teardown hook of its own (it returns a plain
// http.Handler, exactly like every other module's own Mount), so neither
// RateLimiter's background cleanup goroutine is ever explicitly Close()'d --
// both live and leak for the remaining lifetime of the process. This is a
// deliberate, accepted trade-off for this phase's scope, not an oversight:
// NewApp is called exactly ONCE per process (cmd/api/run.go), so this is a
// single, bounded, one-time leak per RateLimiter, functionally equivalent to
// a process-lifetime singleton -- the same lifecycle *db.Pool itself already
// has (closed only via cmd/api/run.go's own top-level defer, never by
// anything internal/platform/app touches). Threading an explicit Close()
// through cmd/api/run.go's own shutdown path would be the more complete
// fix, left for a later phase if/when this service ever needs to
// construct/tear down more than one *http.Handler per process.
func NewApp(cfg config.Config, pool *db.Pool, logger *slog.Logger, verifier *token.Verifier, issuer *token.Issuer, metricsHandler http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	router := httpx.NewRouter(logger, readinessCheck(pool), httpx.DefaultRequestTimeout, metricsHandler)

	// /v1/auth is deliberately OUTSIDE the /v1 group below (which carries
	// auth.Authenticate) -- a login attempt has no token yet. See
	// identityhttp.Mount's own doc comment.
	if pool != nil {
		mountAuthRoutes(router, pool, issuer)
		mountPlatformAdminRoutes(router, pool, cfg.PlatformAdminToken)
	}

	router.Route("/v1/_probe", func(r chi.Router) {
		r.Use(auth.Authenticate(verifier))
		r.With(auth.RequireRole(token.RoleManager)).Post("/events", createProbeEventHandler(pool))
		r.With(auth.RequireRole(token.RoleManager, token.RoleEmployee)).Get("/events/{aggregateID}", getProbeEventHandler(pool))
	})

	router.Route("/v1", func(r chi.Router) {
		r.Use(auth.Authenticate(verifier))
		if pool != nil {
			operationshttp.Mount(r, operationspg.NewUnitOfWork(pool))
			catalighttp.Mount(r, catalogpg.NewUnitOfWork(pool))
			crmhttp.Mount(r, crmpg.NewUnitOfWork(pool))
			communityhttp.Mount(r, communitypg.NewUnitOfWork(pool))

			// ordering is the only module with role-DIFFERENTIATED routes
			// (mesas: manager only; pedidos: manager + employee) -- see
			// internal/modules/ordering/infrastructure/http's own doc
			// comment for why that role restriction is applied HERE, at
			// the composition root, via auth.RequireRole, rather than
			// inside ordering's own Mount (mirroring this file's own
			// /v1/_probe routes just above).
			orderingUow := orderingpg.NewUnitOfWork(pool)
			orderinghttp.MountTables(r.With(auth.RequireRole(token.RoleManager)), orderingUow)
			orderinghttp.MountOrders(r.With(auth.RequireRole(token.RoleManager, token.RoleEmployee)), orderingUow)
		}
	})

	if pool != nil {
		mountOrderingPublicRoutes(router, pool)
	}

	// Wraps the fully-assembled router, rather than router.Use(...) at the
	// top: httpx.NewRouter already registers /healthz, /readyz and /metrics
	// before returning, and chi panics if Use is called after any route has
	// been registered. Wrapping the final http.Handler sidesteps that
	// entirely and applies CORS uniformly to every route above, public and
	// authenticated alike.
	corsMiddleware := cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	})
	return corsMiddleware(router)
}

// mountOrderingPublicRoutes wires internal/modules/ordering's PUBLIC,
// unauthenticated digital-menu routes under /public/v1/{tenantSlug},
// behind -- in this exact order -- (1) a rate limiter keyed by client IP +
// the URL's own (untrusted, not-yet-resolved) tenant slug, so a request
// for a bogus/nonexistent slug is throttled BEFORE it ever costs a
// database round trip, and (2)
// internal/platform/auth/publictenant.Middleware, which resolves that
// slug into a trusted tenant.TenantID -- see
// internal/modules/ordering/infrastructure/httppublic's own doc comment
// for the fuller rationale this mirrors.
func mountOrderingPublicRoutes(router chi.Router, pool *db.Pool) {
	rateLimiter := httpx.NewRateLimiter(publicRateLimitMaxBuckets, publicRateLimitRefillPerSecond, publicRateLimitBurst)

	catalogUow := catalogpg.NewUnitOfWork(pool)
	menuReader := catalogpricing.NewMenuReader(catalogUow)
	priceReader := catalogpricing.NewProductPriceReader(catalogUow)
	orderingUow := orderingpg.NewUnitOfWork(pool)

	router.Route("/public/v1/{tenantSlug}", func(r chi.Router) {
		r.Use(rateLimiter.RateLimit(publicRateLimitKey))
		r.Use(publictenant.Middleware(pool, "tenantSlug"))
		orderinghttppublic.Mount(r, orderingUow, menuReader, priceReader)
	})
}

// publicRateLimitKey builds the rate limiter key for a request against
// /public/v1/{tenantSlug}/...: the client's IP (as resolved by
// internal/platform/httpx.NewRouter's own chimw.ClientIPFromRemoteAddr,
// which always runs earlier in the middleware chain than this route's own
// group -- see httpx.NewRouter's doc comment) combined with the URL's raw
// tenant slug, read directly via chi.URLParam rather than waiting for
// publictenant.Middleware to resolve/validate it (that middleware runs
// AFTER this rate limiter in the chain, precisely so an invalid slug never
// reaches it in the first place once a key is already being throttled).
func publicRateLimitKey(r *http.Request) string {
	return chimw.GetClientIP(r.Context()) + "|" + chi.URLParam(r, "tenantSlug")
}

// mountAuthRoutes wires internal/modules/identity's unauthenticated
// /v1/auth routes behind a RateLimiter keyed by client IP (see
// authRateLimitKey), so POST /v1/auth/login cannot be hammered by an
// automated credential-stuffing attempt -- see authRateLimitBurst's own
// doc comment for why the limit is what it is. This mirrors
// mountOrderingPublicRoutes' shape (a rate limiter built locally, then
// applied via r.Use before the module's own Mount), except keyed by IP
// alone: unlike /public/v1/{tenantSlug}, a login attempt has no tenant
// slug in its URL to combine with the IP.
func mountAuthRoutes(router chi.Router, pool *db.Pool, issuer *token.Issuer) {
	rateLimiter := httpx.NewRateLimiter(authRateLimitMaxBuckets, authRateLimitRefillPerSecond, authRateLimitBurst)

	router.Route("/v1/auth", func(r chi.Router) {
		r.Use(rateLimiter.RateLimit(authRateLimitKey))
		identityhttp.Mount(r, pool, issuer)
	})
}

// authRateLimitKey builds the rate limiter key for a request against
// /v1/auth/...: just the client's IP (as resolved by
// internal/platform/httpx.NewRouter's own chimw.ClientIPFromRemoteAddr --
// see publicRateLimitKey's own doc comment for why that resolution is
// already in place by the time this middleware runs).
func authRateLimitKey(r *http.Request) string {
	return chimw.GetClientIP(r.Context())
}

// mountPlatformAdminRoutes wires internal/platform/platformadmin's
// tenant-provisioning API at /v1/platform, gated by
// platformadmin.RequireToken rather than auth.Authenticate: this API is
// not scoped to any tenant, so a per-tenant JWT has no claim to check it
// against. See internal/platform/platformadmin's own package doc comment
// for the full rationale. No rate limiter is attached here (unlike
// mountAuthRoutes): the token itself is the primary defense, and this
// surface is meant for a small number of trusted platform operators, not
// the general public /v1/auth/login is exposed to.
func mountPlatformAdminRoutes(router chi.Router, pool *db.Pool, platformAdminToken string) {
	router.Route("/v1/platform", func(r chi.Router) {
		r.Use(platformadmin.RequireToken(platformAdminToken))
		r.Route("/tenants", func(r chi.Router) {
			platformadminhttp.Mount(r, pool)
		})
	})
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
