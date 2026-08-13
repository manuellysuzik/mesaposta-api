package publictenant

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const (
	tenantSlugParam = "tenantSlug"
	validTenantUUID = "11111111-1111-1111-1111-111111111111"
)

// probeResult records what the downstream handler actually observed.
type probeResult struct {
	called    bool
	tenantID  string
	tenantErr error
}

func probeHandler(result *probeResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result.called = true
		tid, err := tenant.From(r.Context())
		result.tenantErr = err
		if err == nil {
			result.tenantID = tid.String()
		}
		w.WriteHeader(http.StatusOK)
	}
}

// fakeResolver is an in-memory tenantSlugResolver, letting this package's
// tests exercise resolverMiddleware's full flow without any Postgres/Docker
// dependency.
type fakeResolver struct {
	rawID    string
	err      error
	onLookup func(slug string)
}

func (f fakeResolver) LookupTenantBySlug(_ context.Context, slug string) (string, error) {
	if f.onLookup != nil {
		f.onLookup(slug)
	}
	if f.err != nil {
		return "", f.err
	}
	return f.rawID, nil
}

// requestWithURLParam builds a GET request carrying slug as the chi URL
// param named tenantSlugParam, mirroring how chi.Router itself populates
// chi.RouteContext for a matched {tenantSlug} route -- without needing to
// wire up a real chi.Router just to exercise the middleware in isolation.
// An empty slug leaves the URL param entirely absent, mirroring a route
// that genuinely has no matching path segment.
func requestWithURLParam(slug string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if slug == "" {
		return req
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(tenantSlugParam, slug)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestResolverMiddleware_UnknownOrErroringSlug_Returns404AndNeverCallsHandler(t *testing.T) {
	sentinel := errors.New("boom")

	cases := []struct {
		name     string
		slug     string
		resolver fakeResolver
	}{
		{
			name:     "empty slug",
			slug:     "",
			resolver: fakeResolver{},
		},
		{
			name:     "resolver error (not found)",
			slug:     "mesa-inexistente",
			resolver: fakeResolver{err: sentinel},
		},
		{
			name:     "resolver returns a malformed UUID",
			slug:     "mesa-1",
			resolver: fakeResolver{rawID: "not-a-valid-uuid"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &probeResult{}
			handler := resolverMiddleware(tc.resolver, tenantSlugParam)(probeHandler(result))

			req := requestWithURLParam(tc.slug)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
			}
			if result.called {
				t.Fatalf("downstream handler must never execute when tenant resolution fails")
			}
		})
	}
}

func TestResolverMiddleware_ValidSlug_ExposesExactTenantIDToHandler(t *testing.T) {
	resolver := fakeResolver{rawID: validTenantUUID}
	result := &probeResult{}
	handler := resolverMiddleware(resolver, tenantSlugParam)(probeHandler(result))

	req := requestWithURLParam("mesa-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !result.called {
		t.Fatal("downstream handler should have executed")
	}
	if result.tenantErr != nil {
		t.Fatalf("tenant.From returned error: %v", result.tenantErr)
	}
	if result.tenantID != validTenantUUID {
		t.Fatalf("tenant.From = %q, want %q", result.tenantID, validTenantUUID)
	}
}

func TestResolverMiddleware_ResolverReceivesTrimmedSlugFromURLParam(t *testing.T) {
	var gotSlug string
	resolver := fakeResolver{
		rawID:    validTenantUUID,
		onLookup: func(slug string) { gotSlug = slug },
	}
	handler := resolverMiddleware(resolver, tenantSlugParam)(probeHandler(&probeResult{}))

	req := requestWithURLParam("  mesa-1  ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotSlug != "mesa-1" {
		t.Fatalf("resolver received slug %q, want trimmed %q", gotSlug, "mesa-1")
	}
}

// TestResolverMiddleware_TenantIsolation_DistinctSlugsNeverShareAContext
// guards against any accidental shared mutable state producing the same
// tenant for two different, valid slugs.
func TestResolverMiddleware_TenantIsolation_DistinctSlugsNeverShareAContext(t *testing.T) {
	const otherTenantUUID = "22222222-2222-2222-2222-222222222222"

	resultA := &probeResult{}
	handlerA := resolverMiddleware(fakeResolver{rawID: validTenantUUID}, tenantSlugParam)(probeHandler(resultA))
	recA := httptest.NewRecorder()
	handlerA.ServeHTTP(recA, requestWithURLParam("mesa-a"))

	resultB := &probeResult{}
	handlerB := resolverMiddleware(fakeResolver{rawID: otherTenantUUID}, tenantSlugParam)(probeHandler(resultB))
	recB := httptest.NewRecorder()
	handlerB.ServeHTTP(recB, requestWithURLParam("mesa-b"))

	if resultA.tenantID != validTenantUUID {
		t.Fatalf("request A saw tenant %q, want %q", resultA.tenantID, validTenantUUID)
	}
	if resultB.tenantID != otherTenantUUID {
		t.Fatalf("request B saw tenant %q, want %q", resultB.tenantID, otherTenantUUID)
	}
}
