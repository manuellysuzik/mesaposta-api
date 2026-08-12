// Package http provides the community module's HTTP adapters: chi
// handlers, request/response DTOs, and the wiring into
// internal/platform/httpx (the standard Envelope response shape) and
// internal/platform/auth (Authenticate, and -- unlike
// internal/modules/operations and internal/modules/crm -- also
// auth.ClaimsFromContext, since liking/unliking/commenting need the
// authenticated caller's own identity, not just their tenant; see
// currentuser.go). This package depends on ../../application, never
// directly on ../../domain internals it doesn't need. See
// internal/modules/README.md for how a module's handlers get mounted into
// internal/platform/app.NewApp's router.
package http
