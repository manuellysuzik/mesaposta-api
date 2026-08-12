// Package domain holds the catalog module's entities, value objects,
// domain services, and repository INTERFACES: the sellable Product
// aggregate and the Recipe aggregate a Product can be promoted from. It is
// pure business logic -- see internal/modules/README.md ("Required
// layout" and rule R1) for the full layout this module follows and the
// dependency boundary this package must respect.
//
// Per architecture rule R1 (architecture/rules.go), nothing in this
// package (nor anything it transitively imports) may reach net/http,
// database/sql, pgx, chi, golang-jwt, goose, argon2id, or any
// internal/platform/** package outside the {errs, tenant, outbox}
// allowlist. Repository behavior domain code needs is declared here as an
// interface; the real Postgres-backed implementation lives in
// ../infrastructure/postgres.
//
// This package deliberately does NOT import internal/modules/operations
// (see architecture rule R3): Money, Quantity and ID are each re-declared
// here following the exact same fixed-point/UUID-wrapper pattern
// internal/modules/operations/domain establishes, rather than shared --
// every module owns its own copy of these small, dependency-free value
// objects, exactly as internal/modules/README.md's "modules don't reach
// into each other's internals" boundary requires.
package domain
