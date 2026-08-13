// Package httppublic provides the ordering module's PUBLIC, UNAUTHENTICATED
// HTTP adapter: the digital-menu surface a diner's QR-code scan hits --
// GET /public/v1/{tenantSlug}/cardapio/mesa/{code} (resolve a table,
// mint a TableSession, list the active menu) and
// POST /public/v1/{tenantSlug}/pedidos (submit a cart, requiring a valid
// TableSession token, never a client-supplied price).
//
// This is a package SEPARATE from ../http (never sharing a DTO with it) --
// a deliberate architectural decision, not incidental duplication:
// ../http's responses are allowed to expose management-only fields
// (version, tenant_id, ...) to an AUTHENTICATED, ROLE-CHECKED staff
// member; this package's responses are read by literally anyone who scans
// a QR code, so its own DTOs (dto.go) are built independently, exposing
// only what a diner actually needs. Sharing infra/http's DTOs here would
// mean any future field added there for legitimate management purposes
// (e.g. a table's internal notes, or an order's cost breakdown) would
// silently leak into the unauthenticated response the moment someone
// forgot this distinction -- keeping the packages fully separate makes
// that mistake require an explicit, deliberate choice instead.
//
// Every route Mount registers here is expected to already sit behind, in
// this exact order (see internal/platform/app.NewApp):
//  1. an internal/platform/httpx.RateLimiter, keyed by client IP + tenant
//     slug (BEFORE the tenant is even resolved, so a request for a bogus
//     slug never costs a database round trip);
//  2. internal/platform/auth/publictenant.Middleware, which resolves
//     {tenantSlug} into a trusted tenant.TenantID and attaches it to the
//     request context via tenant.With -- exactly as if it had arrived on a
//     JWT claim.
//
// Mount itself applies neither: it is a pure "wire handlers to routes"
// function, identical in spirit to ../http's own Mount functions.
package httppublic
