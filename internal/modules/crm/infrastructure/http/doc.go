// Package http provides the crm module's HTTP adapters: chi handlers,
// request/response DTOs, and the wiring into internal/platform/httpx (the
// standard Envelope response shape) and internal/platform/tenant (the
// already-authenticated tenant carried on the request context). This
// package depends on ../../application, never directly on ../../domain
// internals it doesn't need. See internal/modules/README.md for how a
// module's handlers get mounted into internal/platform/app.NewApp's
// router.
package http
