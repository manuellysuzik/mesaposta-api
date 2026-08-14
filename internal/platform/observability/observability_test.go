package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/observability"
)

func TestSetup_EmptyEndpoint_DisablesObservability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := observability.Setup(ctx, observability.Config{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("Setup with empty OTLPEndpoint returned an error: %v", err)
	}
	if h == nil {
		t.Fatal("Setup returned a nil Handle for a disabled configuration")
	}
	if h.MetricsHandler != nil {
		t.Fatal("MetricsHandler must be nil when observability is disabled")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := h.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown on a disabled Handle must be a no-op, got error: %v", err)
	}
}

func TestSetup_ZeroValueConfig_DisablesObservability(t *testing.T) {
	// The zero value of Config (as internal/platform/config.Config's own
	// defaults produce when OTEL_EXPORTER_OTLP_ENDPOINT is unset) must be
	// exactly as safe as explicitly passing an empty OTLPEndpoint.
	h, err := observability.Setup(context.Background(), observability.Config{})
	if err != nil {
		t.Fatalf("Setup with a zero-value Config returned an error: %v", err)
	}
	if h.MetricsHandler != nil {
		t.Fatal("MetricsHandler must be nil for a zero-value Config")
	}
}

func TestHandle_Shutdown_NilHandle_IsNoop(t *testing.T) {
	var h *observability.Handle
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown on a nil *Handle must be a no-op, got error: %v", err)
	}
}

func TestSetup_MalformedEndpoint_ReturnsClearError(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"not a URL at all", "::not-a-url::"},
		{"missing scheme and host", "jaeger:4318"},
		{"unsupported scheme", "ftp://jaeger:4318"},
		{"scheme with no host", "http://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			h, err := observability.Setup(ctx, observability.Config{OTLPEndpoint: tt.endpoint})
			if err == nil {
				t.Fatalf("expected an error for malformed endpoint %q, got nil (Handle: %+v)", tt.endpoint, h)
			}
			if h != nil {
				t.Fatalf("expected a nil Handle alongside an error, got %+v", h)
			}
		})
	}
}

func TestSetup_ValidEndpoint_RegistersRealProvidersWithoutNetworkIO(t *testing.T) {
	// otlptracehttp.New does not dial the collector eagerly -- exports are
	// asynchronous/batched -- so this must return quickly and successfully
	// even though nothing is listening on the endpoint. This is exactly
	// the "no Jaeger running in CI" scenario Setup must tolerate.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := observability.Setup(ctx, observability.Config{
		ServiceName:  "test-svc",
		OTLPEndpoint: "http://127.0.0.1:19999",
	})
	if err != nil {
		t.Fatalf("Setup with a well-formed (if unreachable) endpoint returned an error: %v", err)
	}
	if h.MetricsHandler == nil {
		t.Fatal("MetricsHandler must be non-nil once observability is enabled")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := h.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown returned an unexpected error: %v", err)
	}
}
