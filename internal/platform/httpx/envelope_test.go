package httpx_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

func TestEnvelope_JSONShape_Success(t *testing.T) {
	env := httpx.Envelope{
		Data:  map[string]string{"ok": "yes"},
		Error: nil,
		Meta:  httpx.Meta{RequestID: "req-1"},
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"data", "error", "meta"} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("expected top-level field %q in envelope JSON, got: %s", field, raw)
		}
	}
	if string(decoded["error"]) != "null" {
		t.Fatalf("error field = %s, want null", decoded["error"])
	}
}

func TestEnvelope_JSONShape_Error(t *testing.T) {
	env := httpx.Envelope{
		Data:  nil,
		Error: &httpx.ErrorBody{Code: "not_found", Message: "order not found"},
		Meta:  httpx.Meta{RequestID: "req-2"},
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(decoded["data"]) != "null" {
		t.Fatalf("data field = %s, want null", decoded["data"])
	}

	var errBody httpx.ErrorBody
	if err := json.Unmarshal(decoded["error"], &errBody); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if errBody.Code != "not_found" || errBody.Message != "order not found" {
		t.Fatalf("unexpected error body: %+v", errBody)
	}
}

func TestRequestIDContext_RoundTrip(t *testing.T) {
	ctx := httpx.WithRequestID(context.Background(), "abc-123")
	if got := httpx.RequestIDFromContext(ctx); got != "abc-123" {
		t.Fatalf("RequestIDFromContext = %q, want abc-123", got)
	}
}

func TestRequestIDContext_EmptyWhenNeverSet(t *testing.T) {
	if got := httpx.RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("RequestIDFromContext = %q, want empty string", got)
	}
}
