// Package postgres provides the catalog module's Postgres adapters:
// repository IMPLEMENTATIONS satisfying the interfaces declared in
// ../../domain, built on internal/platform/db.Pool.Scope -- the same
// tenant-scoped, row-level-security-enforcing pattern
// internal/platform/outbox/postgres,
// internal/modules/operations/infrastructure/postgres and
// internal/modules/crm/infrastructure/postgres already establish. See
// internal/modules/README.md, "What a module reuses from
// internal/platform/**".
package postgres
