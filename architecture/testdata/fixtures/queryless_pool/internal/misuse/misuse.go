// Package misuse is the fixture's whole reason to exist: it attempts the
// exact thing the real internal/platform/db package is designed to make
// impossible -- running a query against a *db.Pool directly, skipping
// Scope (and therefore skipping the tenant-scoping/RLS guarantee Scope
// exists to enforce). This file must NOT compile.
package misuse

import (
	"context"

	"fixture/queryless_pool/internal/platform/db"
)

// BadQuery tries to call Query directly on a *db.Pool. db.Pool has no
// Query method (see internal/platform/db/pool.go, real and fixture alike),
// so this is a compile error -- exactly the property
// architecture/compilefail_test.go asserts.
func BadQuery(ctx context.Context, pool *db.Pool) {
	_, _ = pool.Query(ctx, "select 1")
}
