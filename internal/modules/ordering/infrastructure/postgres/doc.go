// Package postgres provides the ordering module's Postgres adapters:
// repository IMPLEMENTATIONS satisfying the interfaces declared in
// ../../domain, built on internal/platform/db.Pool.Scope -- the same
// tenant-scoped, row-level-security-enforcing pattern every other module's
// own infrastructure/postgres package already establishes. See
// internal/modules/README.md, "What a module reuses from
// internal/platform/**".
package postgres
