// Package http provides the ordering module's AUTHENTICATED HTTP adapters:
// chi handlers, request/response DTOs, and the wiring into
// internal/platform/httpx (the standard Envelope response shape) and
// internal/platform/tenant (the already-authenticated tenant carried on
// the request context) -- management routes for mesas (tables) and
// pedidos (orders), mounted under /v1 exactly like every other module.
//
// This package depends on ../../application, never directly on
// ../../domain internals it doesn't need.
//
// UNLIKE every other module's Mount in this codebase (crm, community,
// catalog, operations), this package exports TWO Mount functions --
// MountTables and MountOrders (see router.go) -- instead of one, because
// mesas and pedidos routes require DIFFERENT roles (mesas: gestor only;
// pedidos: gestor + funcionario). Neither function applies
// auth.RequireRole itself: internal/platform/app.NewApp, which already
// imports internal/platform/auth for exactly this purpose (see its own
// probe routes), applies the right role to each ahead of time -- see
// router.go's own doc comment for the full reasoning, including why this
// keeps this package's own httptest-based handler tests simple (no need
// to fabricate a signed JWT just to populate auth.ClaimsFromContext).
//
// See ../httppublic for this module's OTHER HTTP adapter -- the
// unauthenticated, tenant-slug-addressed digital-menu surface, which is a
// deliberately SEPARATE package (never sharing a DTO with this one).
package http
