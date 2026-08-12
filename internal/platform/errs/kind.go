// Package errs provides the mesaposta-api structured error taxonomy: a small
// set of stable error "kinds" that map to HTTP-status-like semantics
// (invalid input, unauthenticated, forbidden, not found, conflict, rate
// limited, internal), plus a structured Error type that carries a stable
// machine-readable Code, a client-safe PublicMsg, and the wrapped internal
// cause (kept out of anything sent back to a client).
//
// This package is part of the shared kernel every layer -- including
// internal/domain/** -- is allowed to depend on (see
// architecture/rules.go, R1's platform allowlist). Because of that, it must
// only ever import the standard library (R4): no third-party dependency,
// however small, may leak into it.
package errs

// Kind classifies an error into one of a small, stable set of categories.
// Callers (HTTP handlers, gRPC interceptors, etc.) map a Kind to a transport
// status code; the zero value, KindNone, denotes "no error" and is what
// KindOf returns for a nil error.
type Kind int

const (
	// KindNone is the zero value of Kind. It never appears on a real error;
	// KindOf(nil) returns it to distinguish "there was no error at all"
	// from any real error kind, including Internal.
	KindNone Kind = iota

	// Invalid marks a request that failed validation (bad input shape,
	// missing required field, out-of-range value, etc.).
	Invalid

	// Unauthenticated marks a request missing valid credentials.
	Unauthenticated

	// Forbidden marks a request from an authenticated caller who lacks
	// permission to perform the requested action.
	Forbidden

	// NotFound marks a request referencing a resource that does not exist
	// (or that the caller has no visibility into).
	NotFound

	// Conflict marks a request that could not be completed because it
	// conflicts with the current state of the resource (e.g. a duplicate,
	// or a stale optimistic-concurrency version).
	Conflict

	// TooManyRequests marks a request rejected by rate limiting.
	TooManyRequests

	// Internal marks an unexpected, non-actionable-by-the-client failure.
	// It is also the fallback Kind reported by KindOf for any error that is
	// not a *Error at all (e.g. a raw error from a third-party library),
	// since such an error carries no explicit classification and must be
	// treated as an internal failure rather than silently ignored.
	Internal
)

// String renders a human-readable name for k, primarily for logging.
func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case Invalid:
		return "invalid"
	case Unauthenticated:
		return "unauthenticated"
	case Forbidden:
		return "forbidden"
	case NotFound:
		return "not_found"
	case Conflict:
		return "conflict"
	case TooManyRequests:
		return "too_many_requests"
	case Internal:
		return "internal"
	default:
		return "unknown"
	}
}
