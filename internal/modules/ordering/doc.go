// Package ordering is the ordering module's root: see domain/,
// application/, contracts/ and infrastructure/{http,httppublic,postgres}
// for the actual code. This file exists only so `go doc` has somewhere to
// land for the module as a whole; see internal/modules/README.md for the
// layout every module (including this one) follows.
//
// ordering is the only module in this codebase with TWO HTTP adapters:
// infrastructure/http (authenticated management routes, mounted under
// /v1, exactly like every other module) and infrastructure/httppublic (the
// UNAUTHENTICATED, tenant-slug-addressed digital-menu routes a diner's QR
// code scan hits, mounted under /public/v1/{tenantSlug}). See
// infrastructure/httppublic's own doc comment for why that surface is a
// deliberately SEPARATE package, not a second Mount function bolted onto
// infrastructure/http.
package ordering
