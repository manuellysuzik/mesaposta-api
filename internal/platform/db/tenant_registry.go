package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// lookupTenantBySlugSQL resolves an active tenant's raw id by its public
// slug. is_active is part of the WHERE clause (not just something the
// caller checks afterwards) so a deactivated tenant is indistinguishable,
// from this query's perspective, from one that never existed.
const lookupTenantBySlugSQL = `SELECT id::text FROM tenants WHERE slug = $1 AND is_active`

// LookupTenantBySlug resolves a tenant's raw UUID string by its public
// slug, without requiring a tenant already established in ctx -- this is
// the one deliberate exception to Pool's "no query without Scope" rule,
// used only by internal/platform/auth/publictenant to bootstrap tenant
// identity for unauthenticated, slug-addressed routes (see
// internal/platform/auth/publictenant's doc comment). It returns the raw
// UUID text, never a tenant.TenantID -- constructing that trusted type is
// the caller's job, via tenant.Parse, exactly as if the value had arrived
// on a JWT claim.
func (p *Pool) LookupTenantBySlug(ctx context.Context, slug string) (rawTenantID string, err error) {
	err = p.pool.QueryRow(ctx, lookupTenantBySlugSQL, slug).Scan(&rawTenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Deliberately vague: the caller (publictenant.Middleware)
			// collapses this into the same generic "not found" response an
			// ordering-module handler will later use for an invalid table
			// slug, so no distinction leaks to an unauthenticated caller
			// between "no such slug" and "slug belongs to an inactive
			// tenant". The raw slug itself is not included in the public
			// message.
			return "", errs.New(errs.NotFound, "tenant_not_found", "tenant not found", err)
		}
		return "", errs.New(errs.Internal, "tenant_lookup_failed", "failed to resolve tenant by slug", err)
	}
	return rawTenantID, nil
}
