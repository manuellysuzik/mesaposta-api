-- 0011_users.sql
--
-- Backing table for internal/modules/identity's User aggregate, as
-- persisted by internal/modules/identity/infrastructure/postgres. This is
-- this codebase's FIRST table whose primary lookup path (login, by email)
-- has NO tenant established yet -- the whole point of the lookup is to
-- discover which tenant a caller belongs to -- so it cannot use the plain
-- "tenant_isolation" policy shape every other tenant-scoped table since
-- 0001_baseline.sql uses on its own. See internal/platform/db/user_registry.go's
-- LookupUserCredentialsByEmail for the one Go-level query that exercises
-- the second RLS policy below; every other query against this table (every
-- one issued through internal/platform/db.Pool.Scope, i.e. every write and
-- every read performed by an authenticated request) goes through the first.
--
-- email is UNIQUE GLOBALLY (not UNIQUE (tenant_id, email), unlike
-- customers.email in 0006_crm.sql): a login form submits only an email and
-- a password, with no tenant context of its own -- the email IS how the
-- tenant gets resolved, so two different tenants each having their own
-- "owner@example.com" would make login ambiguous by construction. This is
-- a deliberate, product-level constraint (mirroring how almost every
-- multi-tenant SaaS with email/password login actually works), not an
-- oversight.
--
-- Two permissive RLS policies, combined by OR (Postgres' documented
-- behavior for multiple permissive policies on the same table/command --
-- see https://www.postgresql.org/docs/current/ddl-rowsecurity.html: "if
-- multiple policies apply, they are combined using OR"):
--
--   * tenant_isolation: the ordinary policy shape every other table uses.
--     When the app.tenant_id GUC IS set (every authenticated request that
--     went through internal/platform/db.Pool.Scope), this policy filters
--     rows to that tenant exactly as usual -- and login_lookup, below,
--     evaluates to false in that case (its own USING clause requires the
--     GUC to be UNSET), so it contributes nothing: an authenticated
--     request can NEVER use login_lookup to see another tenant's users.
--     This is the property 0011_users_rls_integration_test.go's
--     TestUsersRLS_TenantIsolationWins... test pins down directly, because
--     it is the one way this table's two-policy design could go wrong
--     silently (a bug that made login_lookup's predicate always true, or
--     forgot the "IS NULL" guard, would turn this table's tenant isolation
--     into a no-op).
--
--   * login_lookup: a SELECT-only policy that is true precisely when NO
--     tenant has been established yet (current_setting('app.tenant_id',
--     true) IS NULL) -- i.e. exactly the one code path
--     (LookupUserCredentialsByEmail) that deliberately runs OUTSIDE
--     Pool.Scope, because establishing that very tenant is its whole job.
--     Every other query against this table always runs through Scope, which
--     always sets the GUC, so login_lookup is always false for them and
--     tenant_isolation alone governs -- login_lookup only ever "activates"
--     for the one unauthenticated, pre-tenant lookup this table exists to
--     support.
--
-- GRANT is SELECT, INSERT, UPDATE only -- no DELETE. A user account is
-- deactivated (is_active = false), never deleted: preserving the row is
-- what lets refresh_tokens.subject_id (see 0003_refresh_tokens.sql) and,
-- eventually, audit_log.actor_id (0010_audit_log.sql) continue to resolve
-- to a real, inspectable account instead of a dangling reference.

-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    email text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    role text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT users_role_check CHECK (role IN ('gestor', 'funcionario'))
);

CREATE INDEX users_tenant_id_idx ON users (tenant_id);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE POLICY login_lookup ON users
    FOR SELECT
    USING (current_setting('app.tenant_id', true) IS NULL);

GRANT SELECT, INSERT, UPDATE ON users TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS users;
