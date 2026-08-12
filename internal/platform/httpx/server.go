package httpx

import (
	"net/http"
	"time"
)

const (
	// DefaultReadHeaderTimeout is the maximum time NewServer allows for
	// reading request headers when ServerConfig.ReadHeaderTimeout is unset,
	// as a baseline defense against slowloris-style attacks.
	DefaultReadHeaderTimeout = 5 * time.Second

	// DefaultMaxRequestBodyBytes is the maximum request body size NewServer
	// enforces when ServerConfig.MaxRequestBodyBytes is unset.
	DefaultMaxRequestBodyBytes int64 = 1 << 20 // 1 MiB
)

// ServerConfig configures NewServer. Addr and Handler are required; the
// timeout/limit fields fall back to the package defaults when left at their
// zero value.
type ServerConfig struct {
	Addr                string
	Handler             http.Handler
	ReadHeaderTimeout   time.Duration
	MaxRequestBodyBytes int64
}

// NewServer builds an *http.Server wired with baseline protections every
// mesaposta-api HTTP entry point needs: a bounded ReadHeaderTimeout (defends
// against slowloris-style slow-header attacks) and a hard cap on request
// body size, applied via http.MaxBytesReader as the outermost wrapper
// around cfg.Handler so it protects every route -- including ones a caller
// adds to the router after NewRouter returns it.
//
// Graceful shutdown is intentionally NOT wrapped here: callers should call
// the returned *http.Server's own Shutdown(ctx) directly, which already
// does exactly what's needed (stop accepting new connections, wait for
// in-flight handlers to finish or ctx to expire).
func NewServer(cfg ServerConfig) *http.Server {
	readHeaderTimeout := cfg.ReadHeaderTimeout
	if readHeaderTimeout <= 0 {
		readHeaderTimeout = DefaultReadHeaderTimeout
	}

	maxBody := cfg.MaxRequestBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxRequestBodyBytes
	}

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           limitRequestBody(maxBody)(cfg.Handler),
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

// limitRequestBody wraps next so every request's body is capped at limit
// bytes via http.MaxBytesReader: a handler that tries to read past the cap
// gets an error from Body.Read instead of the server accepting an unbounded
// body.
func limitRequestBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
