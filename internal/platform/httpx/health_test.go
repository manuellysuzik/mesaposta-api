package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// TestHealthz_Returns200WithoutCallingReadinessCheck relies on Healthz's
// signature itself as the proof: func(http.ResponseWriter, *http.Request)
// accepts no ReadinessCheck at all, so it is structurally impossible for it
// to touch one. This test only exercises the observable behavior: 200,
// no error body, regardless of what dependencies might be unhealthy.
func TestHealthz_Returns200WithoutCallingReadinessCheck(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	httpx.Healthz(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("expected nil error, got %+v", env.Error)
	}
}

func TestReadyz_Returns200WhenDependencyOK(t *testing.T) {
	check := func(context.Context) error { return nil }
	handler := httpx.Readyz(check)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadyz_Returns503WhenDependencyBad(t *testing.T) {
	check := func(context.Context) error { return errors.New("db unreachable") }
	handler := httpx.Readyz(check)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error == nil {
		t.Fatalf("expected non-nil error body")
	}
}
