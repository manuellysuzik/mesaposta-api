// Package domain holds the ordering module's entities, value objects,
// domain services, and repository INTERFACES: Table (a tenant's physical
// table, addressed by a short code printed on/near its QR code), Order (a
// tab opened against one Table, with a fixed set of line items priced at
// creation time and frozen forever after -- see order.go's own doc
// comment), and TableSession (the short-lived opaque token a QR-code scan
// mints, binding an unauthenticated diner's subsequent requests to one
// specific table). It is pure business logic -- see
// internal/modules/README.md ("Required layout" and rule R1) for the full
// layout this module follows and the dependency boundary this package must
// respect.
//
// Per architecture rule R1 (architecture/rules.go), nothing in this package
// (nor anything it transitively imports) may reach net/http, database/sql,
// pgx, chi, golang-jwt, goose, argon2id, or any internal/platform/**
// package outside the {errs, tenant, outbox} allowlist. Repository behavior
// domain code needs is declared here as an interface; the real
// Postgres-backed implementation lives in ../infrastructure/postgres.
//
// This module deliberately does NOT import catalog/contracts (even though
// R3 would allow it): every cross-module type this package's own domain
// service (pricing_service.go) needs is instead a small, local type
// (PriceSnapshot) the application layer translates catalog's
// contracts.ProductPrice into at the boundary -- see pricing_service.go's
// own doc comment for why that extra indirection was chosen deliberately.
package domain
