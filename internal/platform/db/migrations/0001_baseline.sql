-- 0001_baseline.sql
--
-- Establishes the two pieces of infrastructure every later,
-- tenant-scoped table in this codebase (Phases 1-6, plus 0002/0003 in this
-- same block) builds on:
--
--  1. mesaposta_app: the role the application's runtime pool
--     (internal/platform/db.Open) connects as. It is created WITHOUT the
--     BYPASSRLS attribute (Postgres roles are NOBYPASSRLS by default; this
--     is spelled out via NOBYPASSRLS below purely for auditability) and
--     WITHOUT SUPERUSER -- either of which would silently and completely
--     disable row-level security for every table this role touches,
--     regardless of any POLICY defined on it.
--
--     Table OWNERSHIP matters just as much as the role's own attributes:
--     Postgres table owners bypass RLS by default even without BYPASSRLS,
--     unless the table additionally has FORCE ROW LEVEL SECURITY set. This
--     migration (and 0002/0003) run as whatever administrative role runs
--     migrations (see db.Migrate's adminDatabaseURL parameter) -- NEVER as
--     mesaposta_app -- so mesaposta_app is never the owner of a
--     tenant-scoped table. Every such table additionally sets FORCE ROW
--     LEVEL SECURITY as defense in depth against that ownership ever
--     drifting.
--
--     NOTE (dev/test only): the password below is a placeholder used by
--     this repository's own integration tests and local docker-compose
--     setups. Production deployments MUST rotate it out-of-band (ALTER
--     ROLE mesaposta_app WITH PASSWORD '...', via whichever secrets
--     manager owns the real credential) before pointing a real
--     DATABASE_URL at this role. No application code ever reads or writes
--     this literal value.
--
--  2. The "app.tenant_id" convention: internal/platform/db.Scope populates
--     this session-local Postgres setting for the lifetime of a single
--     transaction via:
--
--         SELECT set_config('app.tenant_id', $1, true)
--
--     The third argument (true) is what makes it transaction-LOCAL rather
--     than session-wide -- critical, since pooled connections are reused
--     across different tenants' requests; see
--     internal/platform/db/scope_integration_test.go,
--     TestScope_SetLocalTenantIDDoesNotLeakAcrossPoolReuse. Every
--     tenant-scoped table's RLS policy compares its tenant_id column
--     against current_setting('app.tenant_id', true)::uuid -- the "true"
--     second argument to current_setting makes a missing/unset GUC read as
--     NULL instead of raising an error, so a query that somehow runs
--     outside any Scope sees zero rows (NULL never equals anything) rather
--     than failing loudly or, worse, seeing every tenant's rows.
--
-- scope_probe is a minimal smoke-test table used only by this block's own
-- Scope/RLS tests (internal/platform/db/scope_integration_test.go) -- it is
-- not part of any product domain model.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'mesaposta_app') THEN
        CREATE ROLE mesaposta_app LOGIN PASSWORD 'mesaposta_app_dev_only_change_me'
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE TABLE scope_probe (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    label text NOT NULL
);

ALTER TABLE scope_probe ENABLE ROW LEVEL SECURITY;
ALTER TABLE scope_probe FORCE ROW LEVEL SECURITY;

-- The "tenant_isolation" policy naming convention: every tenant-scoped
-- table in this codebase names its row-level-security policy
-- "tenant_isolation", reusing this exact predicate shape (USING and WITH
-- CHECK both compare tenant_id against the current transaction's
-- app.tenant_id setting). See 0002_outbox.sql and 0003_refresh_tokens.sql
-- for the next two tables that follow this template.
CREATE POLICY tenant_isolation ON scope_probe
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON scope_probe TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS scope_probe;
DROP ROLE IF EXISTS mesaposta_app;
DROP EXTENSION IF EXISTS pgcrypto;
