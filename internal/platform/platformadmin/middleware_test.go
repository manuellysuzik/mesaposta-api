package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/platformadmin"
)

const testToken = "this-is-a-perfectly-fine-32-byte-plus-token-value"

func probeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireToken_MissingHeader_Returns401(t *testing.T) {
	handler := platformadmin.RequireToken(testToken)(probeHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireToken_WrongToken_Returns401(t *testing.T) {
	handler := platformadmin.RequireToken(testToken)(probeHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Platform-Admin-Token", "not-the-right-token-but-still-32-bytes-long")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireToken_CorrectToken_CallsNext(t *testing.T) {
	handler := platformadmin.RequireToken(testToken)(probeHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Platform-Admin-Token", testToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireToken_EmptyHeaderValue_NeverMatchesUnconfiguredToken(t *testing.T) {
	// A misconfigured (empty) server-side token must never be satisfiable
	// by a caller who simply omits the header -- see RequireToken's own doc
	// comment on why the empty-header case is rejected before ever reaching
	// ConstantTimeCompare.
	handler := platformadmin.RequireToken("")(probeHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
