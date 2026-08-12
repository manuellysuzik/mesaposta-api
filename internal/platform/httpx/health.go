package httpx

import (
	"context"
	"net/http"
)

// ReadinessCheck reports whether a dependency (database, cache, ...) is
// currently healthy. Implementations are expected to respect ctx's
// deadline/cancellation rather than blocking indefinitely.
type ReadinessCheck func(ctx context.Context) error

// Healthz answers a liveness probe: it always returns 200 without touching
// any dependency (in particular, it never calls a ReadinessCheck) and must
// never be wired behind an authentication middleware -- orchestrators poll
// it with no credentials.
func Healthz(w http.ResponseWriter, r *http.Request) {
	RespondData(w, r, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

// Readyz builds a readiness-probe handler: it reports 503 (with the
// standard error envelope) when check fails, 200 otherwise. Like Healthz,
// the resulting handler must never require authentication.
func Readyz(check ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := check(r.Context()); err != nil {
			body := ErrorBody{Code: "not_ready", Message: "service is not ready"}
			meta := metaFor(w, r, nil)
			WriteJSON(w, http.StatusServiceUnavailable, Envelope{Data: nil, Error: &body, Meta: meta})
			return
		}
		RespondData(w, r, http.StatusOK, map[string]string{"status": "ready"}, nil)
	}
}
