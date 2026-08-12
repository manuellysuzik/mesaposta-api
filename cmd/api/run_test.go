package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRun_InvalidConfiguration_ReturnsNonZeroExitAndClearStderrMessage(t *testing.T) {
	// Every required variable (DATABASE_URL, JWT_SECRET) is left unset, so
	// config.LoadFrom fails before run ever tries to open a database pool
	// -- this test needs no real Postgres.
	getenv := func(string) string { return "" }

	var stderr, stdout bytes.Buffer
	code := run(context.Background(), getenv, &stderr, &stdout)

	if code == 0 {
		t.Fatalf("run() with missing required env vars: exit code = 0, want non-zero")
	}

	msg := stderr.String()
	if !strings.Contains(msg, "invalid configuration") {
		t.Errorf("stderr = %q, want it to mention %q", msg, "invalid configuration")
	}
	if strings.Contains(msg, "panic") || strings.Contains(msg, "goroutine ") {
		t.Errorf("stderr = %q, must never contain a panic/stack trace", msg)
	}
}

// slowHandler signals startedCh the moment it starts handling a request,
// sleeps for delay (simulating an in-flight, slow request), then responds
// 200 "ok". Used by the graceful-shutdown tests below to have a request
// genuinely in flight at the moment shutdown is triggered.
func slowHandler(startedCh chan<- struct{}, delay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startedCh <- struct{}{}
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServe_GracefulShutdown_WaitsForInFlightRequestToComplete(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	started := make(chan struct{}, 1)
	srv := &http.Server{Handler: slowHandler(started, 150*time.Millisecond)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		resultCh <- serve(ctx, srv, ln, discardLogger(), 2*time.Second)
	}()

	type httpResult struct {
		status int
		body   string
		err    error
	}
	respCh := make(chan httpResult, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			respCh <- httpResult{err: err}
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		respCh <- httpResult{status: resp.StatusCode, body: string(body)}
	}()

	// Wait until the slow handler has actually started before triggering
	// shutdown, so the request is genuinely in flight (not merely queued)
	// when serve begins draining.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the slow request to start")
	}

	cancel() // simulate a SIGTERM/SIGINT delivered into ctx

	select {
	case res := <-respCh:
		if res.err != nil {
			t.Fatalf("in-flight request did not complete successfully during graceful shutdown: %v", res.err)
		}
		if res.status != http.StatusOK || res.body != "ok" {
			t.Fatalf("in-flight request: status=%d body=%q, want 200 \"ok\"", res.status, res.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the in-flight request's response")
	}

	select {
	case code := <-resultCh:
		if code != 0 {
			t.Fatalf("serve() exit code = %d, want 0 (clean graceful shutdown)", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for serve() to return after graceful shutdown")
	}
}

func TestServe_ShutdownTimeoutExceeded_ReturnsNonZero(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	started := make(chan struct{}, 1)
	// The handler sleeps far longer than the shutdown timeout below, so
	// srv.Shutdown must give up and report a timeout instead of waiting
	// forever for it to finish.
	srv := &http.Server{Handler: slowHandler(started, 2*time.Second)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		resultCh <- serve(ctx, srv, ln, discardLogger(), 50*time.Millisecond)
	}()

	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the slow request to start")
	}

	cancel()

	select {
	case code := <-resultCh:
		if code == 0 {
			t.Fatal("serve() exit code = 0, want non-zero when shutdown exceeds its timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for serve() to return after a forced shutdown timeout")
	}
}

func TestRun_MigrateOnBootInProduction_RefusedBeforeOpeningAnyDatabaseConnection(t *testing.T) {
	// APP_ENV=production + MIGRATE_ON_BOOT=true must be refused outright,
	// never silently applying 0001_baseline.sql's dev-only placeholder
	// mesaposta_app password against a production database (see
	// docs/adr/0004 and the MEDIUM finding it closes). DATABASE_URL is a
	// syntactically valid but entirely unreachable DSN: if this test ever
	// regresses to actually dialing it, it will hang/fail for a network
	// reason instead of failing for the right reason, which is itself a
	// useful signal that the guard moved to the wrong place in run().
	env := map[string]string{
		"APP_ENV":         "production",
		"MIGRATE_ON_BOOT": "true",
		"DATABASE_URL":    "postgres://user:pass@127.0.0.1:1/unreachable",
		"JWT_SECRET":      "a-production-grade-secret-that-is-at-least-32-bytes-long",
	}
	getenv := func(k string) string { return env[k] }

	var stderr, stdout bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code := run(ctx, getenv, &stderr, &stdout)

	if code == 0 {
		t.Fatalf("run() with APP_ENV=production and MIGRATE_ON_BOOT=true: exit code = 0, want non-zero")
	}
	msg := stderr.String()
	if !strings.Contains(msg, migrateOnBootEnvVar) || !strings.Contains(msg, "production") {
		t.Errorf("stderr = %q, want it to explain that %s is refused when APP_ENV=production", msg, migrateOnBootEnvVar)
	}
}

func TestMain_UsesOSExitCodeFromRun(t *testing.T) {
	// main() itself is a two-line, untestable-by-design wrapper around
	// run() + os.Exit -- this test exists purely so go test's coverage
	// tooling has some direct evidence main.go's imports/wiring compile
	// and are exercised, without actually calling os.Exit (which would
	// kill the test binary).
	getenv := func(k string) string {
		if k == "APP_ENV" {
			return "development"
		}
		return ""
	}
	var stderr, stdout bytes.Buffer
	code := run(context.Background(), getenv, &stderr, &stdout)
	if code == 0 {
		t.Fatal("run() with missing DATABASE_URL/JWT_SECRET: exit code = 0, want non-zero")
	}
	fmt.Fprint(io.Discard, code) // keep code "used" defensively if assertions above ever change
}
