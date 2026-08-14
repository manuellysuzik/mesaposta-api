package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func alwaysReady(context.Context) error { return nil }

func TestRouter_UnknownRoute_Returns404Envelope(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)

	req := httptest.NewRequest("GET", "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "404 page not found") {
		t.Fatalf("leaked raw chi 404 text: %s", rec.Body.String())
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not a valid envelope: %v (%s)", err, rec.Body.String())
	}
	if env.Error == nil {
		t.Fatalf("expected non-nil error body")
	}
}

func TestRouter_WrongMethodOnExistingRoute_Returns405Envelope(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)
	router.Get("/widgets", func(w http.ResponseWriter, r *http.Request) {
		httpx.RespondData(w, r, 200, map[string]string{"ok": "yes"}, nil)
	})

	req := httptest.NewRequest("POST", "/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 405 {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not a valid envelope: %v (%s)", err, rec.Body.String())
	}
	if env.Error == nil {
		t.Fatalf("expected non-nil error body")
	}
}

func TestRouter_PanicInHandler_Returns500EnvelopeWithoutStackTraceInBody(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)
	router.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("something exploded: secret-detail-xyz")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-detail-xyz") {
		t.Fatalf("panic value leaked into response body: %s", body)
	}
	if strings.Contains(body, "goroutine") {
		t.Fatalf("stack trace leaked into response body: %s", body)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "secret-detail-xyz") {
		t.Fatalf("expected panic value in server log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "goroutine") {
		t.Fatalf("expected stack trace in server log, got: %s", logOutput)
	}
}

func TestRouter_RequestID_GeneratedWhenClientOmitsIt(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)
	router.Get("/echo", func(w http.ResponseWriter, r *http.Request) {
		httpx.RespondData(w, r, 200, nil, nil)
	})

	req := httptest.NewRequest("GET", "/echo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatalf("expected a generated X-Request-ID header")
	}
}

func TestRouter_RequestID_PropagatedWhenClientSendsIt(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)
	router.Get("/echo", func(w http.ResponseWriter, r *http.Request) {
		httpx.RespondData(w, r, 200, nil, nil)
	})

	req := httptest.NewRequest("GET", "/echo", nil)
	req.Header.Set("X-Request-ID", "client-supplied-id")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "client-supplied-id" {
		t.Fatalf("X-Request-ID = %q, want client-supplied-id", got)
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Meta.RequestID != "client-supplied-id" {
		t.Fatalf("meta.request_id = %q, want client-supplied-id", env.Meta.RequestID)
	}
}

func TestRouter_Timeout_ReturnsErrorEnvelopeInsteadOfHanging(t *testing.T) {
	var buf bytes.Buffer
	const configuredTimeout = 30 * time.Millisecond
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, configuredTimeout, nil)
	router.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			httpx.RespondData(w, r, 200, map[string]string{"ok": "too-late"}, nil)
		}
	})

	req := httptest.NewRequest("GET", "/slow", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("request took %v, expected it to return promptly after the configured timeout", elapsed)
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not a valid envelope: %v (%s)", err, rec.Body.String())
	}
	if env.Error == nil {
		t.Fatalf("expected non-nil error body on timeout")
	}
}

func TestRouter_Healthz_MountedAndUnauthenticated(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestNewRouter_ZeroTimeoutFallsBackToDefault(t *testing.T) {
	var buf bytes.Buffer
	// Passing 0 must not panic and must still produce a usable router --
	// exercises NewRouter's "timeout <= 0 -> DefaultRequestTimeout" branch
	// without actually waiting out the (long) default.
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, 0, nil)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequestLogger_HandlesMissingUpstreamRequestID(t *testing.T) {
	var buf bytes.Buffer
	// Exercises RequestLogger's "requestID == ''" branch by calling it
	// directly, bypassing chimw.RequestID (which would otherwise always
	// populate one).
	handler := httpx.RequestLogger(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/no-request-id", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "" {
		t.Fatalf("X-Request-ID header = %q, want empty (no upstream request id middleware ran)", got)
	}
}

func TestRequestLogger_IncludesTraceCorrelationWhenSpanActive(t *testing.T) {
	var buf bytes.Buffer
	handler := httpx.RequestLogger(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	req := httptest.NewRequest("GET", "/traced", nil).WithContext(trace.ContextWithSpanContext(context.Background(), sc))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var logged map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logged); err != nil {
		t.Fatalf("log line is not valid JSON: %v (%s)", err, buf.String())
	}
	if got := logged["trace_id"]; got != traceID.String() {
		t.Errorf("trace_id = %v, want %q", got, traceID.String())
	}
	if got := logged["span_id"]; got != spanID.String() {
		t.Errorf("span_id = %v, want %q", got, spanID.String())
	}
}

func TestRequestLogger_OmitsTraceFieldsWhenNoActiveSpan(t *testing.T) {
	var buf bytes.Buffer
	handler := httpx.RequestLogger(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/untraced", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var logged map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logged); err != nil {
		t.Fatalf("log line is not valid JSON: %v (%s)", err, buf.String())
	}
	if _, ok := logged["trace_id"]; ok {
		t.Errorf("trace_id present in log with no active span: %v (want omitted entirely, not a zero-value id)", logged["trace_id"])
	}
	if _, ok := logged["span_id"]; ok {
		t.Errorf("span_id present in log with no active span: %v (want omitted entirely, not a zero-value id)", logged["span_id"])
	}
}

func TestRouter_Metrics_NotMountedWhenHandlerNil(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, nil)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (no /metrics route when metricsHandler is nil)", rec.Code)
	}
}

func TestRouter_Metrics_MountedWhenHandlerProvided(t *testing.T) {
	var buf bytes.Buffer
	metricsBody := "# HELP fake_metric a fake metric\nfake_metric 1\n"
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(metricsBody))
	})
	router := httpx.NewRouter(testLogger(&buf), alwaysReady, time.Second, metricsHandler)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != metricsBody {
		t.Fatalf("body = %q, want %q", rec.Body.String(), metricsBody)
	}
}

func TestRouter_Readyz_ReflectsReadinessCheck(t *testing.T) {
	var buf bytes.Buffer
	router := httpx.NewRouter(testLogger(&buf), func(context.Context) error {
		return context.DeadlineExceeded
	}, time.Second, nil)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
