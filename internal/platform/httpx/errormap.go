package httpx

import (
	"errors"
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// genericInternalCode and genericInternalMessage are the fixed, client-safe
// {code, message} pair returned for every errs.Internal-kind failure --
// including any error that isn't a classified *errs.Error at all (raw
// stdlib errors, third-party library errors, ...), since those carry no
// safe-to-expose information by construction. This pair is deliberately
// hardcoded and never derived from the failing error, so it can never leak
// an internal cause or an accidentally-sensitive Public() message.
const (
	genericInternalCode    = "internal_error"
	genericInternalMessage = "An internal error occurred. Please try again later."
)

// kindStatus maps every errs.Kind this codebase issues to its HTTP status
// code. errs.KindNone is intentionally absent: it denotes "no error" and
// should never reach here attached to a real error.
var kindStatus = map[errs.Kind]int{
	errs.Invalid:         http.StatusBadRequest,
	errs.Unauthenticated: http.StatusUnauthorized,
	errs.Forbidden:       http.StatusForbidden,
	errs.NotFound:        http.StatusNotFound,
	errs.Conflict:        http.StatusConflict,
	errs.TooManyRequests: http.StatusTooManyRequests,
	errs.Internal:        http.StatusInternalServerError,
}

// StatusFor returns the HTTP status code associated with kind. Any kind not
// present in the table -- including errs.KindNone and any future Kind added
// to errs without a corresponding entry here -- falls back to 500, since an
// unmapped classification must never be treated as anything less severe
// than an internal failure.
func StatusFor(kind errs.Kind) int {
	if status, ok := kindStatus[kind]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// ErrorBodyFor classifies err (via errs.KindOf) and returns the HTTP status
// and the client-safe ErrorBody to send back.
//
// For errs.Internal -- and for any error that doesn't classify as a
// *errs.Error at all, which errs.KindOf also reports as Internal -- the
// returned body is ALWAYS the fixed genericInternalCode/genericInternalMessage
// pair. This holds even for a *errs.Error explicitly constructed with Kind
// Internal and a caller-supplied Public() message: Internal is always
// generic, by design, so no call site can accidentally leak an internal
// detail through a 500 response.
func ErrorBodyFor(err error) (int, ErrorBody) {
	kind := errs.KindOf(err)
	if kind != errs.Internal && kind != errs.KindNone {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return StatusFor(kind), ErrorBody{Code: structured.Code(), Message: structured.Public()}
		}
	}

	return http.StatusInternalServerError, ErrorBody{Code: genericInternalCode, Message: genericInternalMessage}
}

// NotFoundError builds an *errs.Error with Kind errs.NotFound.
//
// This is the ONLY constructor call sites should use for a "resource not
// found" response -- and it MUST be used, with the exact same code and
// publicMsg, for both "the resource genuinely does not exist" and "the
// resource exists but belongs to a different tenant than the caller's".
// cause may carry the real reason (including which tenant actually owns the
// resource) for server-side logs, but ErrorBodyFor never surfaces it: the
// two cases are indistinguishable to the client by construction. See
// respond_test.go, TestRespondError_TenantScopeIndistinguishableFromNotFound.
func NotFoundError(code, publicMsg string, cause error) error {
	return errs.New(errs.NotFound, code, publicMsg, cause)
}

// ForbiddenError builds an *errs.Error with Kind errs.Forbidden, for the
// distinct case of an authenticated, tenant-scoped caller whose role does
// not permit the requested action. Unlike NotFoundError, a Forbidden
// response is allowed to confirm the resource/action exists -- it is only
// the caller's permission that is denied.
func ForbiddenError(code, publicMsg string, cause error) error {
	return errs.New(errs.Forbidden, code, publicMsg, cause)
}
