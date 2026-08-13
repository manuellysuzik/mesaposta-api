-- 0008_tenants.sql
--
-- The tenants table backs internal/platform/db.Pool.LookupTenantBySlug,
-- which resolves a tenant's raw UUID by its public, URL-carried slug for
-- unauthenticated, slug-addressed routes (see
-- internal/platform/auth/publictenant). It is the single table in this
-- schema that deliberately does NOT follow the "tenant_isolation" RLS
-- policy convention every other tenant-scoped table (0001-0007) uses:
-- every one of those tables filters rows BY tenant_id, using a tenant
-- that has already been established in the session. tenants is exactly
-- the table that ESTABLISHES a tenant in the first place -- there is no
-- tenant_id column to filter by, and no app.tenant_id GUC set yet when a
-- lookup-by-slug runs, so a "tenant_isolation"-shaped policy would be
-- both meaningless and self-defeating here.
--
-- Instead:
--   * RLS is still ENABLED and FORCED (defense in depth, and consistency
--     with every other table's baseline posture), but the policy is
--     permissive: USING (true). A tenant's slug is public by definition
--     -- it lives in the URL path of every public route that addresses
--     it -- so there is no row here that needs hiding from any caller.
--   * The GRANT to mesaposta_app is scoped to individual COLUMNS (id,
--     slug, is_active), not the whole table: this is a deliberate
--     blindfold against any sensitive column a later migration might add
--     to this table (e.g. billing details, internal notes) ever becoming
--     readable through the exact same runtime credential that serves
--     public, unauthenticated slug lookups, without that future migration
--     having to remember to carve out an exception.
--   * There is NO INSERT/UPDATE/DELETE grant to mesaposta_app at all:
--     provisioning a tenant is an administrative operation that happens
--     outside the application's runtime request path (e.g. via a
--     migration, an internal admin tool, or a direct psql session using
--     the administrative role), never via a request mesaposta_app
--     services.

-- +goose Up
CREATE TABLE tenants (
    id uuid PRIMARY KEY,
    slug text UNIQUE NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;

-- public_slug_lookup: permissive by design -- see this file's header
-- comment for why this table does not use the "tenant_isolation" policy
-- shape every other table in this schema uses.
CREATE POLICY public_slug_lookup ON tenants
    FOR SELECT
    USING (true);

-- Column-level grant only -- see this file's header comment.
GRANT SELECT (id, slug, is_active) ON tenants TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS tenants;
