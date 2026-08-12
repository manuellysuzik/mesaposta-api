// Package postgres provides the operations module's Postgres adapters:
// repository IMPLEMENTATIONS satisfying the interfaces declared in
// ../../domain, built on internal/platform/db.Pool.Scope -- the same
// tenant-scoped, row-level-security-enforcing pattern
// internal/platform/outbox/postgres and
// internal/platform/auth/refresh.PostgresStore already establish. See
// internal/modules/README.md, "What a module reuses from
// internal/platform/**".
package postgres
