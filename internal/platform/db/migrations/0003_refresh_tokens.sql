-- 0003_refresh_tokens.sql
--
-- Backing table for internal/platform/auth/refresh's Postgres-backed
-- RefreshStore (internal/platform/auth/refresh/postgres_store.go). Follows
-- the same ownership/RLS convention as 0001/0002: owned by whichever
-- administrative role runs migrations (never mesaposta_app), RLS enabled
-- AND forced, policy named "tenant_isolation".
--
-- Every RefreshStore method (including FindByHash, which the in-process
-- refresh.RefreshStore interface deliberately does not parameterize by
-- tenant) is scoped via db.Scope, which requires ctx to already carry the
-- correct tenant.TenantID -- see postgres_store.go's package doc comment
-- for how a caller is expected to establish that tenant before presenting
-- a refresh token (typically derived from the request's host/path, not
-- from the opaque token itself).
--
-- Only the SHA-256 hash of a token's plaintext is ever stored (32 raw
-- bytes, column "hash bytea") -- see refresh.go's own package doc comment.

-- +goose Up
CREATE TABLE refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    hash bytea NOT NULL,
    family_id text NOT NULL,
    subject_id text NOT NULL,
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    rotated_at timestamptz,
    revoked_at timestamptz
);

ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON refresh_tokens
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- Uniqueness on (tenant_id, hash): the mechanism that turns two concurrent
-- rotations of the same presented token into exactly one winner. The
-- adapter's RotateFamily issues the new record's INSERT and the old
-- record's "mark rotated" UPDATE in a single transaction; whichever
-- transaction's INSERT commits first wins outright, and the loser's
-- transaction fails on this constraint (both requests raced to insert an
-- identically-hashed new token derived from the same "already rotated"
-- source, which cannot happen from two independently-generated random
-- tokens -- what this constraint actually guards against is the loser
-- re-observing oldHash as not-yet-rotated and attempting to rotate it a
-- second time; see postgres_store.go for the SELECT ... FOR UPDATE that
-- serializes that race directly).
CREATE UNIQUE INDEX refresh_tokens_tenant_hash_idx ON refresh_tokens (tenant_id, hash);

CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (tenant_id, family_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON refresh_tokens TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
