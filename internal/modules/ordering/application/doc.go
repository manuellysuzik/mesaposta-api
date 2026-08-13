// Package application holds the ordering module's use cases: one
// application service per use case (creating/listing/activating a table,
// creating/confirming/advancing/listing/reading an order), each
// orchestrating ../domain objects and ports (repositories) to fulfil
// exactly one specific use case. See internal/modules/README.md for how
// application/ fits between domain/ and infrastructure/.
//
// Two use cases in this package (ResolveTableForMenu, CreateOrder) are
// deliberately different from every other use case in this codebase so
// far: they back this module's PUBLIC, unauthenticated HTTP surface (see
// ../infrastructure/httppublic), and CreateOrder additionally depends on
// internal/modules/catalog/contracts.ProductPriceReader -- a cross-module
// dependency, permitted by architecture rule R3 (any layer of one module
// may import another module's contracts/ package) -- to resolve real,
// current prices for the requested cart BEFORE ever touching this
// module's own domain.OrderPricingService (domain.PriceOrderLines), so
// that a client-supplied price is never trusted (see CreateOrder's own doc
// comment).
package application
