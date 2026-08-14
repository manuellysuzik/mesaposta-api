package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// RouteTag reads chi's own matched route pattern (e.g.
// "/v1/purchase-orders/{id}") off r's context once the request has been
// routed, and adds it as an "http.route" attribute to the otelhttp.Labeler
// that otelhttp.NewMiddleware installs in context -- which otelhttp then
// folds into both the request's server span AND the
// http.server.request.duration histogram (see
// internal/platform/observability's package doc comment for why that
// histogram is this codebase's source of HTTP p99 latency).
//
// This exists because otelhttp itself only populates "http.route" from Go
// 1.22+ net/http.ServeMux's own r.Pattern field (see otelhttp's internal
// semconv package) -- chi never sets that field. Without RouteTag, every
// request recorded by otelhttp.NewMiddleware would carry an EMPTY
// "http.route", collapsing every route into one indistinguishable metric
// series: a p99 query grouped by route would show a single blended number
// across "/healthz" and "/v1/purchase-orders/{id}" alike, and a p99 query
// NOT grouped by route would be equally useless for finding which specific
// endpoint is slow. Tagging the matched PATTERN (not the raw URL path,
// which would make every distinct id its own metric series -- an
// unbounded-cardinality label) is what makes a per-route p99 breakdown
// possible at all.
//
// RouteTag must run after otelhttp.NewMiddleware in the chain (so a
// Labeler already exists in context to add to -- see NewRouter) and, like
// every middleware here that needs post-routing state, calls
// next.ServeHTTP before reading anything off chi's RouteContext: chi only
// finishes building RouteContext.RoutePattern() once the request has
// actually reached its terminal handler, not before.
func RouteTag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		rctx := chi.RouteContext(r.Context())
		if rctx == nil {
			return
		}
		pattern := rctx.RoutePattern()
		if pattern == "" {
			// No route matched (e.g. the request hit NotFound/
			// MethodNotAllowed) -- leave "http.route" unset rather than
			// tagging it with an empty string, consistent with otelhttp's
			// own behavior when it has no pattern to report.
			return
		}
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(semconv.HTTPRouteKey.String(pattern))
		}
	})
}
