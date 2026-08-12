package httpx_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

func TestNewServer_AppliesDefaultReadHeaderTimeout(t *testing.T) {
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:    ":0",
		Handler: http.NewServeMux(),
	})

	if srv.ReadHeaderTimeout != httpx.DefaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want default %v", srv.ReadHeaderTimeout, httpx.DefaultReadHeaderTimeout)
	}
}

func TestNewServer_HonorsExplicitReadHeaderTimeout(t *testing.T) {
	custom := 2 * time.Second
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:              ":0",
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: custom,
	})

	if srv.ReadHeaderTimeout != custom {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", srv.ReadHeaderTimeout, custom)
	}
}

func TestNewServer_LimitsRequestBodySize(t *testing.T) {
	const limit = 8
	var readErr error

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:                ":0",
		Handler:             handler,
		MaxRequestBodyBytes: limit,
	})

	body := strings.NewReader(strings.Repeat("x", limit*4))
	req, err := http.NewRequest("POST", "http://example.invalid/", body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	rec := &discardResponseWriter{header: http.Header{}}
	srv.Handler.ServeHTTP(rec, req)

	if readErr == nil {
		t.Fatalf("expected a body-too-large error, got nil")
	}
}

// discardResponseWriter is a minimal http.ResponseWriter used where
// httptest.NewRecorder would work equally well, kept local to avoid an
// extra import in this file.
type discardResponseWriter struct {
	header http.Header
	status int
	body   []byte
}

func (d *discardResponseWriter) Header() http.Header { return d.header }
func (d *discardResponseWriter) Write(p []byte) (int, error) {
	d.body = append(d.body, p...)
	return len(p), nil
}
func (d *discardResponseWriter) WriteHeader(status int) { d.status = status }

func TestServer_GracefulShutdown_WaitsForInFlightRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	requestStarted := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	})

	srv := httpx.NewServer(httpx.ServerConfig{Handler: handler})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ln)
	}()

	var (
		clientErr  error
		respStatus int
		respBody   string
	)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			clientErr = err
			return
		}
		defer resp.Body.Close()
		respStatus = resp.StatusCode
		b, _ := io.ReadAll(resp.Body)
		respBody = string(b)
	}()

	<-requestStarted

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)

	<-clientDone
	wg.Wait()

	if shutdownErr != nil {
		t.Fatalf("Shutdown returned error: %v", shutdownErr)
	}
	if clientErr != nil {
		t.Fatalf("in-flight request was cut off instead of completing: %v", clientErr)
	}
	if respStatus != http.StatusOK || respBody != "done" {
		t.Fatalf("unexpected response: status=%d body=%q", respStatus, respBody)
	}
}
