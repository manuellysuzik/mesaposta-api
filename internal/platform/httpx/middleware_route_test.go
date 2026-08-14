package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// mountWithCapturedLabeler builds a chi router carrying an
// otelhttp.Labeler in context (mimicking the one otelhttp.NewMiddleware
// itself installs -- see RouteTag's own doc comment for why RouteTag reads
// it from context rather than owning it) and returns a func exposing
// whatever RouteTag added to it once the request completes.
func mountWithCapturedLabeler(t *testing.T) (r chi.Router, labeler func() *otelhttp.Labeler) {
	t.Helper()
	var captured *otelhttp.Labeler

	r = chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := &otelhttp.Labeler{}
			r = r.WithContext(otelhttp.ContextWithLabeler(r.Context(), l))
			next.ServeHTTP(w, r)
			captured = l
		})
	})
	r.Use(httpx.RouteTag)

	return r, func() *otelhttp.Labeler { return captured }
}

func TestRouteTag_AddsMatchedChiPatternAsHTTPRoute(t *testing.T) {
	r, labeler := mountWithCapturedLabeler(t)
	r.Get("/v1/purchase-orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/v1/purchase-orders/3f1b6c1a-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	attrs := labeler().Get()
	if len(attrs) != 1 {
		t.Fatalf("labeler attrs = %+v, want exactly 1 (http.route)", attrs)
	}
	if got, want := string(attrs[0].Key), "http.route"; got != want {
		t.Errorf("attribute key = %q, want %q", got, want)
	}
	if got, want := attrs[0].Value.AsString(), "/v1/purchase-orders/{id}"; got != want {
		t.Errorf("attribute value = %q, want the TEMPLATED pattern %q, not the raw request path", got, want)
	}
}

func TestRouteTag_NoMatch_LeavesLabelerEmpty(t *testing.T) {
	r, labeler := mountWithCapturedLabeler(t)
	r.Get("/v1/known", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if attrs := labeler().Get(); len(attrs) != 0 {
		t.Errorf("labeler attrs = %+v, want none (no route matched, so http.route must stay unset rather than empty-string)", attrs)
	}
}
