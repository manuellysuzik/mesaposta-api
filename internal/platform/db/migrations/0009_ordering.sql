-- 0009_ordering.sql
--
-- Backing tables for internal/modules/ordering's three aggregates (Table,
-- Order [+ its OrderItem value-object lines]) plus TableSession (the
-- short-lived opaque credential the public cardápio flow mints), as
-- persisted by internal/modules/ordering/infrastructure/postgres. Follows
-- the same ownership/RLS convention as 0001-0007 (0008_tenants.sql is the
-- ONE deliberate exception in this schema, per its own header comment --
-- every table below instead follows the NORMAL convention): every table is
-- owned by whichever administrative role runs migrations (never
-- mesaposta_app), RLS is enabled AND forced, and every policy is named
-- "tenant_isolation" with the exact same predicate used since
-- 0001_baseline.sql:
--
--     USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
--     WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid)
--
-- Cross-tenant foreign keys: tables and orders both carry a
-- UNIQUE (tenant_id, id) constraint, and every child table's foreign key
-- is composite -- FOREIGN KEY (tenant_id, <col>) REFERENCES <parent>
-- (tenant_id, id) -- instead of a bare FOREIGN KEY (<col>) REFERENCES
-- <parent> (id), for the identical reason documented in
-- 0004_operations.sql: a single-column FK would let a row reference a
-- same-"id" row belonging to a DIFFERENT tenant.
--
-- order_items.product_id carries NO foreign key -- deliberately, and for a
-- DIFFERENT reason than 0004/0005's own "no FK yet, the aggregate doesn't
-- exist" placeholders: it references a Product owned by a DIFFERENT
-- MODULE (catalog), which DOES already have a real products table (see
-- 0005_catalog.sql) this migration could technically reference. This
-- migration deliberately does NOT add that FK anyway: a hard database-level
-- foreign key crossing a module boundary would silently couple
-- ordering's own schema to catalog's exact table name/shape, undermining
-- the same module independence architecture rule R3 enforces at the Go
-- import-graph level (internal/modules/README.md) -- modules integrate
-- with each other exclusively through <module>/contracts, never through
-- shared database schema. Price/availability integrity for order_items is
-- instead enforced entirely at the application layer, BEFORE any row here
-- is ever written (see internal/modules/ordering/application.CreateOrder
-- and domain.PriceOrderLines).
--
-- version supports optimistic concurrency on tables and orders (see
-- internal/modules/ordering/domain/ports.go's own UpdateWithVersion doc
-- comments). order_items and table_sessions are both append-only (an
-- Order's line items never change after creation; a TableSession is
-- immutable once minted, it simply expires), so both are granted SELECT
-- and INSERT only, mirroring purchase_order_items/stock_movements'
-- identical treatment in 0004_operations.sql.

-- +goose Up

-- tables: a tenant's physical table, addressed both by authenticated staff
-- (management CRUD) and by an unauthenticated diner scanning its QR code
-- (via code -- see domain/table.go). code is unique PER TENANT.
CREATE TABLE tables (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    code text NOT NULL,
    label text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT tables_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT tables_tenant_code_key UNIQUE (tenant_id, code)
);

ALTER TABLE tables ENABLE ROW LEVEL SECURITY;
ALTER TABLE tables FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON tables
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tables TO mesaposta_app;

-- orders: a tab opened against one table (see domain/order.go). Every
-- order in this phase is implicitly "a pagar na mesa depois" -- there is
-- NO payment/billing column anywhere in this schema, per this module's
-- confirmed product-scope decision. customer_name/customer_phone are both
-- nullable and are only ever populated by the public cardápio flow.
--
-- The status CHECK mirrors internal/modules/ordering/domain.OrderStatus's
-- closed set exactly -- a second, physical backstop behind the
-- application-level ParseOrderStatus validation that type already
-- performs.
CREATE TABLE orders (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    table_id uuid NOT NULL,
    customer_name text NULL,
    customer_phone text NULL,
    status text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT orders_status_check
        CHECK (status IN ('pendente_confirmacao', 'confirmado', 'em_preparo', 'pronto', 'entregue', 'cancelado')),
    CONSTRAINT orders_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT orders_table_fk
        FOREIGN KEY (tenant_id, table_id) REFERENCES tables (tenant_id, id)
);

ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON orders
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON orders TO mesaposta_app;

-- order_items: OrderItem, a value object with no identity of its own
-- outside the Order that owns it (see domain/order_item.go) -- hence the
-- surrogate "id" primary key (needed for the row to have one at all) is
-- never read back by application code, only ever generated by the
-- database default. unit_price is numeric(14,2) (whole currency units, 2
-- decimal places), the exact same monetary scale/convention
-- 0004_operations.sql and 0005_catalog.sql already establish --
-- internal/modules/ordering/domain.Money stores CENTS as an int64, and the
-- Postgres adapter converts inline in its SQL ("$n::numeric / 100" on the
-- way in, "(column * 100)::bigint" on the way out), same as every other
-- Money column in this schema. quantity is a plain bigint (a whole-number
-- count, never a float -- see domain.Quantity's own doc comment): unlike
-- 0004/0005's stock/ingredient quantities, there is no fractional scale to
-- convert, mirroring loyalty_points' identical treatment in
-- 0006_crm.sql.
--
-- This table is never updated after insert (Order.Items() never changes
-- once the order is created -- no status transition touches it), so it is
-- granted SELECT and INSERT only.
CREATE TABLE order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    order_id uuid NOT NULL,
    product_id uuid NOT NULL, -- no FK: references catalog.products, a DIFFERENT module -- see this file's own header comment.
    quantity bigint NOT NULL CHECK (quantity > 0),
    unit_price numeric(14,2) NOT NULL CHECK (unit_price > 0),
    CONSTRAINT order_items_order_fk
        FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);

ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON order_items
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON order_items TO mesaposta_app;

-- table_sessions: the short-lived, opaque credential a QR-code scan mints
-- (see domain/table_session.go), binding an otherwise-unauthenticated
-- diner's subsequent POST .../pedidos request to exactly one (tenant,
-- table) pair. Only the SHA-256 hash of a session token's plaintext is
-- ever persisted here (token_hash bytea, 32 bytes), mirroring
-- refresh_tokens' identical "hash only, never plaintext" treatment in
-- 0003_refresh_tokens.sql -- WITHOUT that table's rotation
-- (family_id/rotated_at/revoked_at) columns, since this module's confirmed
-- product decision is that a table session needs no rotation lineage, just
-- "does it exist, is it unexpired, which table does it point at".
--
-- Immutable once inserted (a session simply expires, per its own
-- expires_at), so granted SELECT and INSERT only.
CREATE TABLE table_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    table_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT table_sessions_table_fk
        FOREIGN KEY (tenant_id, table_id) REFERENCES tables (tenant_id, id),
    CONSTRAINT table_sessions_tenant_token_hash_key UNIQUE (tenant_id, token_hash)
);

ALTER TABLE table_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE table_sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON table_sessions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON table_sessions TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS table_sessions;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS tables;
