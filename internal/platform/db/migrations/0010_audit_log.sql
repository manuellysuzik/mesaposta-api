-- 0010_audit_log.sql
--
-- Backing table for internal/platform/audit's Postgres-backed Recorder
-- (internal/platform/audit/postgres/postgres_recorder.go): the
-- cross-module audit trail recording who did what, to which entity, when,
-- and what the relevant state looked like before and after.
--
-- Follows the same ownership/RLS convention every migration since
-- 0001_baseline.sql establishes: owned by whichever administrative role
-- runs migrations (never mesaposta_app), RLS enabled AND forced, and a
-- policy named "tenant_isolation" with the exact same predicate:
--
--     USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
--     WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid)
--
-- Column-shape decisions worth their own note (see
-- docs/adr/0006-audit-log.md for the full rationale):
--
--   * actor_id is NULLABLE TEXT, not a NOT NULL uuid. There is no identity
--     module in this system yet, so the only "who" the application can
--     observe is the authenticated access token's "sub" claim (see
--     internal/platform/auth.ClaimsFromContext), which is not guaranteed
--     to be a UUID and is not backed by any table this schema could point
--     a foreign key at. It is nullable because not every action has a
--     human actor: a scheduled job, a webhook replay or a data migration
--     legitimately has none, and NULL states that unambiguously instead
--     of inviting a sentinel value that would be indistinguishable from a
--     real actor id.
--
--   * entity_type / entity_id are TEXT, not an enum + uuid. This table is
--     deliberately cross-module: it records purchase orders, stock items,
--     products, orders, customers -- entities whose id types live in five
--     different bounded contexts, none of which the shared kernel may
--     know about (architecture rule R3). A closed enum here would force a
--     migration every time any module audits a new kind of entity, which
--     is exactly the friction that makes teams skip auditing.
--
--   * before_data / after_data are NULLABLE jsonb. A creation has no
--     "before"; a deletion has no meaningful "after". They hold small
--     snapshots of the RELEVANT fields (e.g. {"status", "version"}), not
--     full entity dumps -- the trail must stay cheap enough to always
--     write, and must not become a second, stale copy of every aggregate.

-- +goose Up

-- audit_log: an append-only trail. GRANT SELECT, INSERT only --
-- deliberately omitting UPDATE and DELETE -- is this table's own
-- enforcement of that immutability at the one layer application code
-- cannot talk its way around, exactly as stock_movements and transactions
-- (0004_operations.sql) already do: even a bug, or a future engineer who
-- "just this once" adds an UpdateEntry method, can never issue an UPDATE
-- or DELETE against this table as the mesaposta_app role, because the
-- privilege to do so was never granted in the first place. An audit trail
-- that the audited application can rewrite is not an audit trail.
CREATE TABLE audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    actor_id text NULL,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    before_data jsonb NULL,
    after_data jsonb NULL,
    occurred_at timestamptz NOT NULL
);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON audit_log
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- The query this table exists to answer: "what happened to THIS entity?".
-- (tenant_id, entity_type, entity_id) is the access path for an entity's
-- own history; tenant_id leads so the index is useful under the RLS
-- predicate, which always constrains it.
CREATE INDEX audit_log_tenant_entity_idx ON audit_log (tenant_id, entity_type, entity_id);

GRANT SELECT, INSERT ON audit_log TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS audit_log;
