# ADR-0007: Trace/Log Correlation, Real Per-Route p99, and DB Pool Gauges

**Status**: Accepted
**Date**: 2026-08-14
**Author**: Manuelly Suzik
**Context**: Follow-up to the tracing/metrics platform wiring landed alongside `internal/platform/observability` (see that package's own doc comment) and `docs/adr/0006-audit-log.md`'s audit rollout. This ADR covers the gaps found while turning "traces and metrics exist" into "traces and metrics are actually usable for finding a slow request."

## Problem

`internal/platform/observability.Setup` already wires a real OTel `TracerProvider`/`MeterProvider` (OTLP/HTTP export + Prometheus exposition), and `otelhttp.NewMiddleware` already wraps every request in a server span. That is necessary but not sufficient for the stated goal (audit + traces + **p99 metrics**) for three concrete reasons found while validating it end-to-end rather than assuming it worked:

1. **No trace↔log correlation.** `httpx.RequestLogger`'s structured log line had no `trace_id`/`span_id`. An operator staring at a slow request in the logs had no way to jump to the matching trace in Jaeger — only a timestamp to guess by.
2. **`http.route` was always empty.** `otelhttp` only populates its `http.route` span/metric attribute from Go 1.22+ `net/http.ServeMux`'s own `r.Pattern` field (see `otelhttp`'s internal `semconv` package). This codebase routes with `chi`, which never sets that field. Without a fix, every request recorded by `otelhttp.NewMiddleware` — and therefore every data point in the `http.server.request.duration` histogram this ADR's whole point is to make useful — would carry an **empty** `http.route`. A p99 query grouped by route would show one blended number across `/healthz` and `/v1/purchase-orders/{id}` alike; a p99 query not grouped by route would be equally useless for finding which endpoint is actually slow.
3. **No visibility into DB connection pool saturation.** Pool exhaustion (a request queued waiting for a pooled connection) shows up as HTTP latency, not database latency — it is a common, otherwise-invisible root cause of p99 tail latency, and nothing exported it.

## Decision

### `httpx.RouteTag` — the actual fix that makes per-route p99 possible

A new middleware, `internal/platform/httpx.RouteTag` (`middleware_route.go`), mounted immediately after `otelhttp.NewMiddleware` in `NewRouter`'s fixed chain. It reads chi's own matched route pattern off `chi.RouteContext(r.Context()).RoutePattern()` — available only once chi has actually dispatched the request, so it runs `next.ServeHTTP` first, then reads it — and adds it to the `otelhttp.Labeler` already in context via `otelhttp.LabelerFromContext`. `otelhttp` itself folds `Labeler.Get()` into both the span's attributes and the recorded histogram's attributes (see `otelhttp`'s own `handler.go`), so this one middleware fixes `http.route` for both traces and metrics at once.

Tags the TEMPLATED pattern (`/v1/purchase-orders/{id}`), never the raw URL path — the raw path would make every distinct id its own metric series, an unbounded-cardinality label that is worse than no label at all.

### Trace/log correlation

`httpx.RequestLogger` and `httpx.Recoverer` (`middleware_log.go`) now read `trace.SpanContextFromContext(ctx)` and add `trace_id`/`span_id` to their structured log line whenever a valid span is active — via a small shared `traceLogAttrs` helper so both of this package's log-emission points do it identically. When observability is disabled (no OTel provider registered), `SpanContextFromContext` returns an invalid context and the fields are omitted entirely, never logged as a zero-value id that could be mistaken for a real one.

### DB pool gauges

`observability.RegisterPoolGauges` registers two OTel asynchronous (observable) gauges — `db.client.connection.count` (split by the `db.client.connections.state` attribute: `used`/`idle`) and `db.client.connection.max` — following the OTel semconv DB-client-connection-pool conventions (`go.opentelemetry.io/otel/semconv/v1.26.0`), sourced by calling a caller-supplied `func() PoolStats` on every metrics scrape. Wired from `cmd/api/run.go` right after `db.Open`, reading `*pgxpool.Stat`. `observability` itself stays agnostic of `pgx` — the same design principle `db.WithQueryTracer` already established (composing a concrete dependency is left to the caller that already imports it).

### Grafana, provisioned

`docker-compose.yml` gains a `grafana` service (dev-only, anonymous viewer access, no login screen), provisioned automatically via `deploy/grafana/provisioning/` (a Prometheus datasource pointed at the existing `prometheus` service) and `deploy/grafana/dashboards/mesaposta-api.json` — a starter dashboard with p50/p95/p99 latency (overall and by route), request rate by route, 5xx error rate, and DB pool saturation.

The dashboard's PromQL was **not** written from assumption: the OTel→Prometheus metric/label name translation (`http.server.request.duration` seconds histogram → `http_server_request_duration_seconds_bucket/_sum/_count`, attribute `http.route` → label `http_route`, etc.) was verified by actually running the `otelprom` exporter against a synthetic sample and reading its real exposition output before the queries were written.

## Rationale: why `RouteTag` is a separate middleware, not a change to `otelhttp.NewMiddleware`'s call

`otelhttp` already ships the exact mechanism this needs (`WithRouteTag`, `Labeler`) — the gap is purely that chi never populates the one field (`r.Pattern`) `otelhttp`'s own route-tagging logic reads by default. Reimplementing or forking `otelhttp` was never on the table; `RouteTag` is ~30 lines that bridges chi's routing metadata into the mechanism `otelhttp` already provides, keeping this codebase's own `otelhttp` usage exactly as documented upstream everywhere except this one interop gap.

## Related Files

- `internal/platform/httpx/middleware_route.go`, `middleware_route_test.go` — `RouteTag`
- `internal/platform/httpx/middleware_log.go`, `router_test.go` — trace/log correlation
- `internal/platform/observability/observability.go`, `observability_test.go` — `RegisterPoolGauges`
- `cmd/api/run.go` — pool gauge wiring
- `docker-compose.yml`, `deploy/grafana/` — Grafana service + provisioning + dashboard

## See Also

- `internal/platform/observability`'s own package doc comment — the base tracing/metrics wiring this ADR builds on
- [ADR-0006: Audit Log](0006-audit-log.md) — the audit half of this same observability effort, and the staged-rollout precedent `BACKLOG.md` item 7 follows
