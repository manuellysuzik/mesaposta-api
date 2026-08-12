package httpx_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

func TestRespondData_IncludesRequestIDInHeaderAndMeta(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(httpx.WithRequestID(req.Context(), "req-123"))
	rec := httptest.NewRecorder()

	httpx.RespondData(rec, req, 200, map[string]string{"hello": "world"}, nil)

	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Fatalf("X-Request-ID header = %q, want req-123", got)
	}

	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Meta.RequestID != "req-123" {
		t.Fatalf("meta.request_id = %q, want req-123", env.Meta.RequestID)
	}
	if env.Error != nil {
		t.Fatalf("expected nil error, got %+v", env.Error)
	}
	if !strings.Contains(rec.Body.String(), `"error":null`) {
		t.Fatalf("expected literal null error field in JSON, got: %s", rec.Body.String())
	}
}

func TestRespondError_500NeverLeaksInternalCause(t *testing.T) {
	sensitive := "connection string: postgres://user:senha@host:5432/db"
	cause := errors.New(sensitive)
	err := errs.New(errs.Internal, "db_error", "a public safe message", cause)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	httpx.RespondError(rec, req, err)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "senha") || strings.Contains(body, "postgres://") {
		t.Fatalf("response body leaked sensitive content: %s", body)
	}
}

func TestRespondError_RawErrorAlsoMapsTo500WithoutLeaking(t *testing.T) {
	sensitive := errors.New("connection string: postgres://user:senha@host/db")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	httpx.RespondError(rec, req, sensitive)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "senha") {
		t.Fatalf("response body leaked sensitive content: %s", rec.Body.String())
	}
}

// TestRespondError_TenantScopeIndistinguishableFromNotFound proves the RFC
// rule "tenant scope mismatch -> 404 indistinguishable from genuinely not
// found, role/permission failure -> 403": a handler that uses
// httpx.NotFoundError with the same code/publicMsg for both "resource
// genuinely does not exist" and "resource exists but belongs to a different
// tenant" produces byte-identical response bodies for the two cases -- no
// oracle lets a client tell them apart.
func TestRespondError_TenantScopeIndistinguishableFromNotFound(t *testing.T) {
	genuinelyMissing := httpx.NotFoundError("order_not_found", "order not found", nil)
	wrongTenant := httpx.NotFoundError("order_not_found", "order not found",
		errors.New("order 42 belongs to tenant B, request came from tenant A"))

	req1 := httptest.NewRequest("GET", "/orders/42", nil)
	req1 = req1.WithContext(httpx.WithRequestID(req1.Context(), "same-id"))
	rec1 := httptest.NewRecorder()
	httpx.RespondError(rec1, req1, genuinelyMissing)

	req2 := httptest.NewRequest("GET", "/orders/42", nil)
	req2 = req2.WithContext(httpx.WithRequestID(req2.Context(), "same-id"))
	rec2 := httptest.NewRecorder()
	httpx.RespondError(rec2, req2, wrongTenant)

	if rec1.Code != 404 || rec2.Code != 404 {
		t.Fatalf("expected both 404, got %d and %d", rec1.Code, rec2.Code)
	}
	if rec1.Body.String() != rec2.Body.String() {
		t.Fatalf("bodies differ:\n%s\nvs\n%s", rec1.Body.String(), rec2.Body.String())
	}
}

func TestRespondError_ForbiddenIsDistinctFromNotFound(t *testing.T) {
	forbidden := httpx.ForbiddenError("insufficient_role", "you do not have permission", nil)

	req := httptest.NewRequest("GET", "/orders/42", nil)
	rec := httptest.NewRecorder()
	httpx.RespondError(rec, req, forbidden)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "order_not_found") {
		t.Fatalf("forbidden body unexpectedly resembles not-found body: %s", rec.Body.String())
	}
}

func TestRespondError_FallsBackToHeaderRequestIDWhenContextHasNone(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "from-header")
	rec := httptest.NewRecorder()

	httpx.RespondError(rec, req, errors.New("boom"))

	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Meta.RequestID != "from-header" {
		t.Fatalf("meta.request_id = %q, want from-header", env.Meta.RequestID)
	}
}
