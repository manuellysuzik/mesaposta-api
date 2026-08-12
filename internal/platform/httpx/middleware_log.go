package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

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
				logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered",
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.Any("panic", rec),
					slog.String("stack", string(stack)),
				)

				RespondError(w, r, errs.New(errs.Internal, "panic_recovered",
					"an unexpected error occurred", nil))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
