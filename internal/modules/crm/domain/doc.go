// Package domain holds the crm module's entities, value objects, domain
// services, and repository INTERFACES: customers and their loyalty ledger
// (LoyaltyEntry, a child entity nested inside the Customer aggregate -- see
// customer.go's own doc comment for why that aggregate boundary was chosen
// deliberately). It is pure business logic -- see internal/modules/README.md
// ("Required layout" and rule R1) for the full layout this module follows
// and the dependency boundary this package must respect.
//
// Per architecture rule R1 (architecture/rules.go), nothing in this package
// (nor anything it transitively imports) may reach net/http, database/sql,
// pgx, chi, golang-jwt, goose, argon2id, or any internal/platform/**
// package outside the {errs, tenant, outbox} allowlist. Repository behavior
// domain code needs is declared here as an interface; the real
// Postgres-backed implementation lives in ../infrastructure/postgres.
package domain
