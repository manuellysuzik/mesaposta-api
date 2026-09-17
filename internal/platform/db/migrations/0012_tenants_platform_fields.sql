-- 0012_tenants_platform_fields.sql
--
-- Extends tenants (0008_tenants.sql) with the fields and write access the
-- platform-admin-only tenant provisioning API
-- (internal/platform/platformadmin) needs: owner contact details, a
-- billing plan, and a richer status than is_active alone captures.
--
-- This deliberately reverses part of 0008_tenants.sql's original design --
-- see docs/adr/0008-platform-tenant-provisioning.md for the full
-- rationale. The permissive INSERT/UPDATE policies below mirror 0008's own
-- "public_slug_lookup ... USING (true)" policy for the same reason: there
-- is no tenant_id column on tenants to filter by (it IS the table that
-- defines tenants), so real authorization for these writes is enforced
-- entirely by internal/platform/platformadmin.RequireToken at the HTTP
-- layer, not by RLS.
--
-- id and slug are granted INSERT (a tenant's id/slug are set once, at
-- creation) but deliberately NOT UPDATE: slug is embedded in every
-- /public/v1/{tenantSlug}/... URL a diner may have bookmarked/shared, so it
-- (and the primary key itself) must never change after creation.
--
-- SELECT is extended to cover every column ListTenants/CreateTenant's
-- RETURNING/UpdateTenantDetails' RETURNING/SetTenantStatus's RETURNING
-- need to read back -- 0008's own SELECT grant only ever covered
-- (id, slug, is_active), the minimum the public slug-lookup path needs.

-- +goose Up
ALTER TABLE tenants
    ADD COLUMN owner_name text,
    ADD COLUMN owner_email text,
    ADD COLUMN phone text,
    ADD COLUMN plan text NOT NULL DEFAULT 'basic'
        CONSTRAINT tenants_plan_check CHECK (plan IN ('basic', 'pro', 'enterprise')),
    ADD COLUMN status text NOT NULL DEFAULT 'trial'
        CONSTRAINT tenants_status_check CHECK (status IN ('trial', 'active', 'suspended', 'canceled'));

CREATE POLICY platform_admin_insert ON tenants
    FOR INSERT
    WITH CHECK (true);

CREATE POLICY platform_admin_update ON tenants
    FOR UPDATE
    USING (true)
    WITH CHECK (true);

GRANT INSERT (id, slug, name, owner_name, owner_email, phone, plan, status, is_active, created_at) ON tenants TO mesaposta_app;
GRANT UPDATE (name, owner_name, owner_email, phone, plan, status, is_active) ON tenants TO mesaposta_app;
GRANT SELECT (name, owner_name, owner_email, phone, plan, status, created_at) ON tenants TO mesaposta_app;

-- +goose Down
REVOKE SELECT (name, owner_name, owner_email, phone, plan, status, created_at) ON tenants FROM mesaposta_app;
REVOKE INSERT, UPDATE ON tenants FROM mesaposta_app;

DROP POLICY IF EXISTS platform_admin_insert ON tenants;
DROP POLICY IF EXISTS platform_admin_update ON tenants;

ALTER TABLE tenants
    DROP COLUMN owner_name,
    DROP COLUMN owner_email,
    DROP COLUMN phone,
    DROP COLUMN plan,
    DROP COLUMN status;
