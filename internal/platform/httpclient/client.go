// Package httpclient is the standard OUTBOUND *http.Client this codebase's
// composition root (or any future module's infrastructure layer) should
// build any external HTTP integration on -- a payment gateway, a delivery
// partner's API, anything reached over the network from inside this
// process. No module makes an outbound HTTP call yet (see
// internal/modules/README.md's module layout: every module today is
// Postgres-only), but when one does, it MUST go through NewClient rather
// than http.DefaultClient or a hand-rolled &http.Client{}: NewClient's
// Transport is wrapped with otelhttp.NewTransport, so every outbound
// request automatically becomes a child span of whatever inbound request
// triggered it (propagating trace context via the standard
// traceparent/tracestate headers) and shows up in the same trace as the
// rest of that request's work -- exactly like internal/platform/httpx's own
// server-side otelhttp middleware does for inbound requests. When
// observability is disabled (see internal/platform/observability's package
// doc comment), otel's global TracerProvider is a no-op, so this wrapping
// costs nothing beyond a few no-op interface calls.
package httpclient

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultTimeout bounds a request made through a client NewClient returns
// when the caller does not override it via WithTimeout. Every outbound
// call this codebase makes is expected to be a synchronous, user-facing
// dependency (a payment authorization, a delivery quote, ...) -- an
// unbounded default here would let a single slow/unresponsive third party
// stall a request indefinitely.
const DefaultTimeout = 10 * time.Second

// Option configures the *http.Client NewClient builds.
type Option func(*http.Client)

// WithTimeout overrides the client's overall request timeout (see
// DefaultTimeout). A non-positive d is ignored, leaving the previous
// timeout (DefaultTimeout, unless an earlier Option already changed it)
// in place.
func WithTimeout(d time.Duration) Option {
	return func(c *http.Client) {
		if d > 0 {
			c.Timeout = d
		}
	}
}

// NewClient builds an *http.Client whose Transport is wrapped with
// otelhttp.NewTransport (see the package doc comment for why), with
// DefaultTimeout applied unless overridden via WithTimeout.
func NewClient(opts ...Option) *http.Client {
	c := &http.Client{
		Timeout:   DefaultTimeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
