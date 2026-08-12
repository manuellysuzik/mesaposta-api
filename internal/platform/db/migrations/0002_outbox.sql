-- 0002_outbox.sql
--
-- Backing table for internal/platform/outbox's Postgres-backed
-- DomainEventPublisher (internal/platform/outbox/postgres_publisher.go).
-- Follows the same ownership/RLS convention 0001_baseline.sql establishes:
-- owned by whichever administrative role runs migrations (never
-- mesaposta_app), RLS enabled AND forced, policy named "tenant_isolation".

-- +goose Up
CREATE TABLE domain_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    event_name text NOT NULL,
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL,
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0
);

ALTER TABLE domain_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE domain_events FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON domain_events
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- Ordering guarantee internal/platform/outbox's Postgres adapter relies on:
-- (tenant_id, occurred_at, id) gives every tenant's events a stable,
-- deterministic replay order even when several events share an
-- occurred_at timestamp (id, a uuid, breaks ties consistently).
CREATE INDEX domain_events_tenant_order_idx ON domain_events (tenant_id, occurred_at, id);

-- Partial index over unpublished events only: this is the query a
-- background relay/dispatcher will run repeatedly (Phase 1+), and it
-- should never have to scan already-published rows to find pending work.
CREATE INDEX domain_events_unpublished_idx ON domain_events (tenant_id, occurred_at, id)
    WHERE published_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON domain_events TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS domain_events;
