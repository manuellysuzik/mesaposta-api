// Package http provides the identity module's HTTP adapters: the login
// handler and its request/response DTOs, wired into internal/platform/httpx
// (the standard Envelope response shape). Unlike every other module's own
// infrastructure/http package, Mount here registers routes OUTSIDE the
// auth.Authenticate-protected /v1 group -- a login attempt has no token yet
// -- see router.go's own doc comment.
package http
