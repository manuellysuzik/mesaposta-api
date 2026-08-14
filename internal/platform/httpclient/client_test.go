package httpclient_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpclient"
)

func TestNewClient_DefaultsTimeoutAndWrapsTransport(t *testing.T) {
	c := httpclient.NewClient()

	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.Timeout != httpclient.DefaultTimeout {
		t.Fatalf("Timeout = %v, want %v", c.Timeout, httpclient.DefaultTimeout)
	}
	if c.Transport == nil {
		t.Fatal("Transport is nil, want an otelhttp-wrapped transport")
	}
}

func TestNewClient_WithTimeout_Overrides(t *testing.T) {
	c := httpclient.NewClient(httpclient.WithTimeout(2 * time.Second))

	if c.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", c.Timeout)
	}
}

func TestNewClient_WithTimeout_IgnoresNonPositive(t *testing.T) {
	c := httpclient.NewClient(httpclient.WithTimeout(0), httpclient.WithTimeout(-1))

	if c.Timeout != httpclient.DefaultTimeout {
		t.Fatalf("Timeout = %v, want default %v (non-positive overrides must be ignored)", c.Timeout, httpclient.DefaultTimeout)
	}
}

func TestNewClient_UsableForARealRequest(t *testing.T) {
	c := httpclient.NewClient(httpclient.WithTimeout(2 * time.Second))

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	// Connecting to a closed local port must fail fast (not hang, not
	// panic) -- this exercises the otelhttp-wrapped Transport actually
	// round-tripping a real *http.Request rather than merely compiling.
	if _, err := c.Do(req); err == nil {
		t.Fatal("expected a connection error against a closed local port, got nil")
	}
}
