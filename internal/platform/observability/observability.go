// Package observability wires mesaposta-api's OpenTelemetry tracing and
// metrics. Setup builds (or, when unconfigured, deliberately skips) the
// process-wide TracerProvider and MeterProvider that every other platform
// package reaches through OpenTelemetry's own global API --
// internal/platform/db's otelpgx-backed pgx.QueryTracer (see
// internal/platform/db.WithQueryTracer), internal/platform/httpx's
// otelhttp server middleware, and internal/platform/httpclient's
// otelhttp-wrapped outbound *http.Client all read spans/meters from
// go.opentelemetry.io/otel's global providers rather than being handed a
// provider instance directly. That means Setup only needs to run once, at
// process start (cmd/api/run.go), strictly before anything else that might
// emit a span or record a metric -- in particular before
// internal/platform/db.Open, so the pool's own QueryTracer picks up a real
// (or, see below, no-op) provider from the moment the first connection is
// established.
//
// Setup is deliberately a no-op by default: when Config.OTLPEndpoint is
// empty (the zero value, and internal/platform/config's own default), it
// registers no exporter at all and leaves otel's global providers at their
// built-in no-op implementation -- every span/metric call anywhere else in
// this codebase then costs nothing beyond a few no-op interface calls, with
// no goroutines started and no network I/O attempted. This is CRITICAL for
// `go test ./...`: CI has no Jaeger/collector running, and must never block
// on, retry against, time out on, or otherwise slow down because of an
// OTLP endpoint that does not exist. Every test in this package exercises
// only the empty-endpoint and malformed-endpoint paths for exactly that
// reason -- see observability_test.go.
package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// meterName identifies this package as the OTel instrumentation scope for
// the gauges RegisterPoolGauges creates -- the same role
// defaultServiceName plays for the resource-level "service.name", but at
// the instrumentation-library level (see otel.Meter's own doc comment).
const meterName = "github.com/manuellysuzik/mesaposta-api/internal/platform/observability"

// serviceNameKey is the OTel resource attribute key otel's own
// semconv.ServiceNameKey constant resolves to ("service.name"). It is
// spelled out directly here, rather than importing
// go.opentelemetry.io/otel/semconv/<version>, so this package does not need
// to track semconv's own versioned-package churn for the sake of a single
// well-known, stable attribute key.
const serviceNameKey = "service.name"

// defaultServiceName is used when Config.ServiceName is empty, so a caller
// that only sets OTLPEndpoint still gets identifiable traces/metrics
// instead of an empty "service.name".
const defaultServiceName = "mesaposta-api"

// Config configures Setup.
type Config struct {
	// ServiceName identifies this process in exported traces/metrics (the
	// OTel "service.name" resource attribute). Defaults to
	// "mesaposta-api" when empty.
	ServiceName string
	// OTLPEndpoint is the OTLP/HTTP collector this process exports traces
	// to (e.g. "http://jaeger:4318" -- see otlptracehttp.WithEndpointURL).
	// Empty disables observability entirely: see the package doc comment.
	OTLPEndpoint string
}

// Handle is what Setup returns.
type Handle struct {
	// MetricsHandler serves Prometheus-exposition-format metrics scraped
	// from the MeterProvider Setup registered, for
	// internal/platform/httpx.NewRouter to mount at /metrics. Nil when
	// observability is disabled (Config.OTLPEndpoint was empty) -- callers
	// MUST treat a nil MetricsHandler as "do not register the route at
	// all" (see httpx.NewRouter's own doc comment for that contract),
	// never as "serve an empty handler".
	MetricsHandler http.Handler

	shutdownFuncs []func(context.Context) error
}

// Shutdown calls, in order, every teardown Setup registered -- in
// particular the TracerProvider's own Shutdown, which flushes any spans
// not yet exported and closes the OTLP connection -- collecting every
// error encountered rather than stopping at the first one, so a failure
// tearing down tracing never prevents metrics teardown from being
// attempted too.
//
// Shutdown is always safe to call, including on a Handle Setup returned
// for a disabled configuration (Config.OTLPEndpoint == "") and on a nil
// *Handle: both are no-ops.
func (h *Handle) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}
	var errs []error
	for _, fn := range h.shutdownFuncs {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Setup configures OpenTelemetry tracing and metrics for this process and
// registers them as otel's global providers (otel.SetTracerProvider /
// otel.SetMeterProvider) -- see the package doc comment for why every
// other platform package deliberately reaches its tracer/meter through
// those globals instead of importing this package directly.
//
// When cfg.OTLPEndpoint is empty, Setup does nothing beyond returning a
// Handle whose Shutdown is a no-op and whose MetricsHandler is nil: the
// global providers are left at otel's own no-op implementation. This is
// the default (internal/platform/config.Config.OTELExporterOTLPEndpoint
// defaults to ""), so a deployment that never sets
// OTEL_EXPORTER_OTLP_ENDPOINT gets zero observability overhead.
//
// When cfg.OTLPEndpoint is set but malformed (not a valid absolute
// http(s) URL), Setup returns a clear, non-nil error instead of silently
// falling back to a default endpoint or panicking -- otlptracehttp's own
// WithEndpointURL silently keeps its built-in default on an invalid URL,
// which would hide a misconfiguration, so the URL is validated here first.
func Setup(ctx context.Context, cfg Config) (*Handle, error) {
	if cfg.OTLPEndpoint == "" {
		return &Handle{}, nil
	}

	if err := validateEndpoint(cfg.OTLPEndpoint); err != nil {
		return nil, fmt.Errorf("observability: invalid OTLPEndpoint %q: %w", cfg.OTLPEndpoint, err)
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String(serviceNameKey, serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("observability: building resource: %w", err)
	}

	tracerProvider, err := setupTracing(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	meterProvider, metricsHandler, err := setupMetrics(res)
	if err != nil {
		// Best-effort: nothing has been registered as a global provider
		// yet at this point (otel.SetTracerProvider happens after both
		// providers are built successfully, below), so there is no
		// partial global state to unwind -- just release the tracer
		// provider's own resources before returning the error.
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	return &Handle{
		MetricsHandler: metricsHandler,
		shutdownFuncs: []func(context.Context) error{
			tracerProvider.Shutdown,
			meterProvider.Shutdown,
		},
	}, nil
}

// validateEndpoint reports a clear error for anything that is not an
// absolute http(s) URL, rather than letting otlptracehttp.WithEndpointURL
// silently discard it and fall back to its own default.
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	return nil
}

// setupTracing builds a batching TracerProvider exporting to
// cfg.OTLPEndpoint over OTLP/HTTP. WithInsecure is used deliberately: the
// collector this points at (e.g. Jaeger, in the docker-compose topology
// this service runs in) is reached over a private compose/VPC network, not
// the public internet, so plaintext OTLP is an accepted trade-off here in
// exchange for not having to provision and rotate a TLS cert for
// telemetry's internal hop.
func setupTracing(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: building OTLP trace exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	), nil
}

// setupMetrics builds a MeterProvider backed by a Prometheus exposition
// reader, along with the http.Handler that serves it. It registers the
// exporter's collector on a dedicated *prometheus.Registry rather than
// prometheus/client_golang's global DefaultRegisterer: this package's own
// tests build multiple MeterProviders across test cases, and registering
// against the shared global registerer would make a second Setup call in
// the same process fail with a duplicate-collector error.
func setupMetrics(res *resource.Resource) (*sdkmetric.MeterProvider, http.Handler, error) {
	registry := prometheus.NewRegistry()

	exporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, nil, fmt.Errorf("observability: building Prometheus exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return meterProvider, handler, nil
}

// PoolStats is the subset of a connection pool's own stats (e.g.
// *pgxpool.Stat, see internal/platform/db.Pool.Stat) RegisterPoolGauges
// reports as OTel gauges, following the db.client.connection.*
// semantic conventions (go.opentelemetry.io/otel/semconv/v1.26.0). It is a
// plain struct rather than an interface over *pgxpool.Stat itself so this
// package does not need to import pgx -- see the package doc comment for
// why observability deliberately stays agnostic of any one concrete
// dependency it instruments; composing RegisterPoolGauges with a real pool
// is left to a caller that already imports pgx, such as cmd/api/run.go.
type PoolStats struct {
	// UsedConns is the number of connections currently acquired (in use).
	UsedConns int64
	// IdleConns is the number of connections currently idle in the pool.
	IdleConns int64
	// MaxConns is the maximum number of connections the pool may open.
	MaxConns int64
}

// RegisterPoolGauges registers OTel asynchronous gauges reporting a
// connection pool's used/idle/max connection counts
// (db.client.connection.count, split by the db.client.connections.state
// attribute, and db.client.connection.max), sourced by calling statFn on
// every metrics scrape rather than on a timer -- consistent with
// internal/platform/db.Pool.Stat() itself being a live snapshot, not
// something continuously pushed.
//
// These are DB connection pool metrics, not HTTP ones, but pool
// exhaustion is a common root cause of HTTP p99 tail latency: a request
// queued waiting for a pooled connection shows up as HTTP latency, not
// database latency. Exporting them alongside http.server.request.duration
// (see internal/platform/httpx.RouteTag for how that histogram gets a
// usable per-route "http.route" label) is what lets that be diagnosed from
// a dashboard instead of guessed at.
//
// Safe to call even when observability is disabled (Setup was never
// called, or was called with an empty OTLPEndpoint): otel's global
// MeterProvider is then its own no-op implementation, so every gauge
// registered here is simply never invoked. Also safe to call before or
// after Setup: otel's global Meter is a delegating proxy that binds to
// whichever MeterProvider is current at the time metrics are actually
// collected, not at instrument-creation time.
func RegisterPoolGauges(statFn func() PoolStats) error {
	meter := otel.Meter(meterName)

	count, err := meter.Int64ObservableGauge(
		semconv.DBClientConnectionCountName,
		metric.WithUnit(semconv.DBClientConnectionCountUnit),
		metric.WithDescription(semconv.DBClientConnectionCountDescription),
	)
	if err != nil {
		return fmt.Errorf("observability: building %s gauge: %w", semconv.DBClientConnectionCountName, err)
	}

	max, err := meter.Int64ObservableGauge(
		semconv.DBClientConnectionMaxName,
		metric.WithUnit(semconv.DBClientConnectionMaxUnit),
		metric.WithDescription(semconv.DBClientConnectionMaxDescription),
	)
	if err != nil {
		return fmt.Errorf("observability: building %s gauge: %w", semconv.DBClientConnectionMaxName, err)
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stats := statFn()
		o.ObserveInt64(count, stats.UsedConns, metric.WithAttributes(semconv.DBClientConnectionsStateUsed))
		o.ObserveInt64(count, stats.IdleConns, metric.WithAttributes(semconv.DBClientConnectionsStateIdle))
		o.ObserveInt64(max, stats.MaxConns)
		return nil
	}, count, max)
	if err != nil {
		return fmt.Errorf("observability: registering db pool gauges callback: %w", err)
	}
	return nil
}
