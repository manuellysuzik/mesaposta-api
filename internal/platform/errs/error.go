package errs

import (
	"errors"
	"fmt"
)

// Error is the structured error type every layer of mesaposta-api should
// return instead of a bare error, once the failure is classifiable. It
// carries:
//   - Kind: the stable category used to pick a transport status code.
//   - Code: a stable, machine-readable string (e.g. "invalid_argument")
//     intended for the frontend/API consumer to branch on.
//   - a client-safe public message, returned by Public().
//   - the internal cause, wrapped via fmt.Errorf's %w so it remains
//     reachable through errors.Is/errors.As/errors.Unwrap for logging and
//     debugging, but is never exposed by Public().
type Error struct {
	kind      Kind
	code      string
	publicMsg string
	err       error // wraps cause via %w; nil when constructed without a cause
}

// New builds a structured *Error. cause may be nil (e.g. for validation
// errors with no deeper underlying error to wrap); when non-nil, it is
// wrapped with fmt.Errorf's %w verb so callers can still use errors.Is /
// errors.As / errors.Unwrap to inspect it -- while Public() never surfaces
// it.
func New(kind Kind, code, publicMsg string, cause error) *Error {
	var wrapped error
	if cause != nil {
		wrapped = fmt.Errorf("%s: %w", code, cause)
	}
	return &Error{
		kind:      kind,
		code:      code,
		publicMsg: publicMsg,
		err:       wrapped,
	}
}

// Error implements the error interface. The returned string is intended for
// server-side logs only -- it includes the wrapped internal cause (if any)
// and therefore may contain sensitive details. Use Public() for anything
// that could reach a client.
func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.publicMsg, e.err)
	}
	return e.publicMsg
}

// Unwrap exposes the wrapped internal cause (if any) to the standard
// errors.Is / errors.As / errors.Unwrap machinery.
func (e *Error) Unwrap() error {
	return e.err
}

// Kind returns the error's classification.
func (e *Error) Kind() Kind {
	return e.kind
}

// Code returns the stable, machine-readable error code.
func (e *Error) Code() string {
	return e.code
}

// Public returns the client-safe message. It NEVER includes the wrapped
// internal cause, regardless of what that cause contains.
func (e *Error) Public() string {
	return e.publicMsg
}

// KindOf classifies err:
//   - nil returns KindNone.
//   - an err whose chain (per errors.As, so it sees through any number of
//     fmt.Errorf("...: %w", ...) wrappers) contains a *Error returns that
//     Error's Kind.
//   - any other non-nil error (no *Error anywhere in its chain) is treated
//     as an unclassified failure and returns Internal.
func KindOf(err error) Kind {
	if err == nil {
		return KindNone
	}
	var structured *Error
	if errors.As(err, &structured) {
		return structured.kind
	}
	return Internal
}

// IsKind reports whether err classifies (per KindOf) as kind.
func IsKind(err error, kind Kind) bool {
	return KindOf(err) == kind
}
