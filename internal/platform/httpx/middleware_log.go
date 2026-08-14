package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// RequestLogger builds the structured-access-log middleware: it copies the
// request id chi's RequestID middleware placed in its own context key into
// this package's context key (see requestid.go) and onto the X-Request-ID
// response header, then logs one structured line per request via logger,
// including request_id and -- when the request has already been
// authenticated by the time this line is emitted -- tenant_id.
//
// When the request carries a valid OTel span (i.e. observability is
// enabled -- see internal/platform/observability's package doc comment --
// and this middleware runs, per NewRouter's fixed ordering, AFTER the
// otelhttp server span has already started), the log line also carries
// trace_id and span_id, read from trace.SpanContextFromContext. This is
// what lets an operator go from a log line straight to the matching trace
// in Jaeger, and back: without it, correlating the two requires guessing
// by timestamp. A request with no active span (observability disabled, or
// a test that never wired otelhttp) omits both fields rather than logging
// the zero trace/span ID, which would look like a real (if empty-looking)
// identifier instead of "no trace was ever started here".
//
// This middleware must run after chimw.RequestID (so a request id already
// exists to copy) and before Recoverer/RequestTimeout in the chain, per the
// fixed ordering RequestID -> RealIP -> log -> Recoverer -> Timeout.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := chimw.GetReqID(r.Context())
			if requestID != "" {
				w.Header().Set(requestIDHeader, requestID)
			}
			r = r.WithContext(WithRequestID(r.Context(), requestID))

			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			attrs := []slog.Attr{
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}
			if tid, err := tenant.From(r.Context()); err == nil {
				attrs = append(attrs, slog.String("tenant_id", tid.String()))
			}
			attrs = append(attrs, traceLogAttrs(r.Context())...)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request", attrs...)
		})
	}
}

// Recoverer builds the panic-recovery middleware: it recovers any panic
// from the handlers further down the chain, logs the panic value and a full
// stack trace via logger (server-side only), and writes the standard 500
// error envelope -- which, per ErrorBodyFor's Internal-kind handling, never
// contains the panic value or the stack trace.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				stack := debug.Stack()
				attrs := []slog.Attr{
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.Any("panic", rec),
					slog.String("stack", string(stack)),
				}
				attrs = append(attrs, traceLogAttrs(r.Context())...)
				logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered", attrs...)

				RespondError(w, r, errs.New(errs.Internal, "panic_recovered",
					"an unexpected error occurred", nil))
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// traceLogAttrs returns trace_id/span_id log attributes for the OTel span
// active in ctx, or nil when ctx carries no valid span (observability
// disabled, or no otelhttp middleware ran) -- see RequestLogger's doc
// comment for why the zero trace/span ID must never be logged as if it
// were a real one. Shared between RequestLogger and Recoverer so both of
// this package's structured log emission points stay correlatable to
// Jaeger the same way.
func traceLogAttrs(ctx context.Context) []slog.Attr {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	}
}
