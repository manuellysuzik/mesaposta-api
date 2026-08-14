package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultRequestTimeout is used by NewRouter whenever the caller-supplied
// timeout is zero or negative.
const DefaultRequestTimeout = 30 * time.Second

// NewRouter builds the chi.Router every mesaposta-api HTTP entry point
// shares: the fixed middleware chain
//
//	otelhttp span -> RouteTag -> RequestID -> RealIP -> structured log -> Recoverer -> Timeout
//
// (see middleware_log.go for the log/Recoverer steps, middleware_route.go
// for RouteTag, and RequestTimeout below), a chi-native-text-free 404/405
// envelope, and the unauthenticated /healthz and /readyz probes. Callers
// add their own routes on the returned router (e.g. router.Get("/v1/...",
// ...)) after wrapping them in whatever auth middleware
// (internal/platform/auth) a given route needs.
//
// The otelhttp step wraps every request in a server span via
// otelhttp.NewMiddleware, reading the process-wide TracerProvider
// registered by internal/platform/observability.Setup (see that package's
// own doc comment) through OpenTelemetry's global API -- it runs FIRST in
// the chain so the span covers request ID assignment, logging and every
// downstream handler. When Setup was never called (or was called with
// observability disabled), otel's global TracerProvider is its own no-op
// implementation, so this middleware costs a few no-op interface calls per
// request and never blocks or fails.
//
// RouteTag runs immediately after: see its own doc comment for why, without
// it, otelhttp's http.route attribute (and therefore every route-grouped
// p99 query against http.server.request.duration) would be empty for every
// request this chi-routed service ever serves.
//
// metricsHandler, when non-nil, is mounted at GET /metrics (unauthenticated,
// alongside /healthz and /readyz) -- typically
// internal/platform/observability.Handle.MetricsHandler, serving
// Prometheus-exposition-format metrics. A nil metricsHandler (observability
// disabled, or a caller/test that does not need it) means NO /metrics route
// is registered at all, rather than one serving an empty body.
//
// The "RealIP" step uses chimw.ClientIPFromRemoteAddr rather than the
// deprecated chimw.RealIP: chimw.RealIP blindly trusts client-supplied
// True-Client-IP/X-Real-IP/X-Forwarded-For headers and mutates
// r.RemoteAddr from them, which lets any client spoof its logged IP (see
// GHSA-3fxj-6jh8-hvhx). ClientIPFromRemoteAddr never trusts request
// headers; it exposes the TCP-layer address via chimw.GetClientIP. Once
// this service's actual reverse-proxy topology is known, this can be
// swapped for chimw.ClientIPFromXFFTrustedProxies with the right trusted
// proxy count.
func NewRouter(logger *slog.Logger, ready ReadinessCheck, timeout time.Duration, metricsHandler http.Handler) chi.Router {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}

	r := chi.NewRouter()
	r.Use(otelhttp.NewMiddleware("mesaposta-api"))
	r.Use(RouteTag)
	r.Use(chimw.RequestID)
	r.Use(chimw.ClientIPFromRemoteAddr)
	r.Use(RequestLogger(logger))
	r.Use(Recoverer(logger))
	r.Use(RequestTimeout(timeout))

	r.NotFound(notFoundHandler)
	r.MethodNotAllowed(methodNotAllowedHandler)

	r.Get("/healthz", Healthz)
	r.Get("/readyz", Readyz(ready))
	if metricsHandler != nil {
		r.Handle("/metrics", metricsHandler)
	}

	return r
}

// notFoundHandler replaces chi's default "404 page not found" plain-text
// body with the standard error envelope, using the same NotFoundError
// constructor (and therefore the same body shape) any handler uses for a
// missing resource.
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	RespondError(w, r, NotFoundError("route_not_found", "the requested resource was not found", nil))
}

// methodNotAllowedHandler replaces chi's default 405 plain-text body with
// the standard error envelope. 405 has no corresponding errs.Kind (it is a
// transport-level routing concern, not a domain classification), so the
// envelope is built directly rather than through ErrorBodyFor.
func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	body := ErrorBody{Code: "method_not_allowed", Message: "method not allowed for this resource"}
	meta := metaFor(w, r, nil)
	WriteJSON(w, http.StatusMethodNotAllowed, Envelope{Data: nil, Error: &body, Meta: meta})
}

// RequestTimeout builds a middleware that cancels the request's context
// after d and, if the wrapped handler has not returned by then, writes the
// standard error envelope with status 504 instead of chi's blank
// middleware.Timeout response.
//
// Like chi's own middleware.Timeout (this is a deliberate, documented port
// of that same cooperative-cancellation model -- see
// github.com/go-chi/chi/v5/middleware/timeout.go), this does NOT forcibly
// abort a handler that ignores ctx.Done(): a well-behaved handler MUST
// select on <-r.Context().Done() and return promptly for the timeout to
// actually bound wall-clock latency. This keeps the implementation free of
// the goroutine-leak and response-writer-race hazards a hard-abort
// implementation (à la net/http.TimeoutHandler) would introduce, at the
// cost of relying on handlers to cooperate -- the same trade-off chi itself
// makes.
func RequestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer func() {
				cancel()
				if ctx.Err() == context.DeadlineExceeded {
					body := ErrorBody{Code: "request_timeout", Message: "the request took too long to process"}
					meta := metaFor(w, r, nil)
					WriteJSON(w, http.StatusGatewayTimeout, Envelope{Data: nil, Error: &body, Meta: meta})
				}
			}()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
