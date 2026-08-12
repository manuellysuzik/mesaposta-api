// Package httpx provides mesaposta-api's shared HTTP transport plumbing:
// a standard JSON response envelope (envelope.go), the mapping from the
// domain error taxonomy (internal/platform/errs) to HTTP status codes and
// client-safe error bodies (errormap.go), helpers to write that envelope to
// an http.ResponseWriter (respond.go), request-id context propagation
// (requestid.go), the router/middleware chain, health endpoints and the
// http.Server wiring (router.go, server.go, middleware_log.go, health.go).
//
// The single most important guarantee this package enforces: an
// errs.Internal-kind failure (or any error that isn't a classified
// *errs.Error at all) NEVER surfaces its internal cause -- or even a
// caller-supplied Public() message -- to an HTTP client. See errormap.go's
// ErrorBodyFor and respond_test.go's non-leak tests.
package httpx
