-- 0004_operations.sql
--
-- Backing tables for internal/modules/operations' four aggregates
-- (StockItem, PurchaseOrder [+ its PurchaseOrderItem value objects],
-- StockMovement, Transaction), as persisted by
-- internal/modules/operations/infrastructure/postgres. Follows the same
-- ownership/RLS convention as 0001/0002/0003: every table is owned by
-- whichever administrative role runs migrations (never mesaposta_app), RLS
-- is enabled AND forced, and every policy is named "tenant_isolation" with
-- the exact same predicate used since 0001_baseline.sql:
--
--     USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
--     WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid)
--
-- Cross-tenant foreign keys: every parent table below (stock_items,
-- purchase_orders) additionally carries a UNIQUE (tenant_id, id)
-- constraint, and every child table's foreign key is composite --
-- FOREIGN KEY (tenant_id, <col>) REFERENCES <parent> (tenant_id, id) --
-- instead of a bare FOREIGN KEY (<col>) REFERENCES <parent> (id). A
-- single-column FK would let a row reference a same-"id" row belonging to
-- a DIFFERENT tenant (ids are globally unique UUIDs, so nothing else would
-- stop that); the composite FK makes a cross-tenant reference physically
-- impossible to insert, not just something RLS happens to hide afterwards.
--
-- Enum CHECK constraints mirror internal/modules/operations/domain/enums.go
-- exactly (see that file for the closed sets StockUnit, StockMovementType,
-- PurchaseOrderStatus, TransactionType and TransactionCategory validate
-- against) -- a second, physical backstop behind the application-level
-- Parse<Enum> validation every one of those types already performs.
--
-- Monetary columns are numeric(14,2) (whole currency units, 2 decimal
-- places); quantity columns are numeric(14,3) (whole units, 3 decimal
-- places). internal/modules/operations/domain.Money stores CENTS as an
-- int64, and domain.Quantity stores THOUSANDTHS of a unit as an int64 --
-- the Postgres adapter converts between the two representations inline in
-- its SQL ("$n::numeric / 100" / "$n::numeric / 1000" on the way in,
-- "(column * 100)::bigint" / "(column * 1000)::bigint" on the way out)
-- rather than storing raw cents/thousandths directly, so that a DBA
-- inspecting this schema with a plain "SELECT * FROM stock_items" sees
-- ordinary, human-readable decimal amounts.

-- +goose Up

-- stock_items: an ingredient/supply tracked in a tenant's stock (see
-- domain/stock_item.go). supplier_id is deliberately a bare uuid with NO
-- foreign key: the Supplier aggregate does not exist in this phase, so
-- there is nothing yet for it to reference -- it exists purely so a future
-- phase can backfill the FK (and, at that point, the composite
-- (tenant_id, supplier_id) -> suppliers(tenant_id, id) shape every other
-- cross-aggregate reference in this file already follows) without a
-- column rename.
CREATE TABLE stock_items (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    unit text NOT NULL,
    quantity numeric(14,3) NOT NULL CHECK (quantity >= 0),
    min_quantity numeric(14,3) NOT NULL CHECK (min_quantity >= 0),
    cost_per_unit numeric(14,2) NOT NULL CHECK (cost_per_unit >= 0),
    supplier_id uuid NULL, -- no FK: the Supplier aggregate does not exist in this phase.
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL,
    CONSTRAINT stock_items_unit_check CHECK (unit IN ('kg', 'g', 'l', 'ml', 'un')),
    CONSTRAINT stock_items_tenant_id_id_key UNIQUE (tenant_id, id)
);

ALTER TABLE stock_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_items FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON stock_items
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON stock_items TO mesaposta_app;

-- purchase_orders: an order placed with a supplier for one or more stock
-- items (see domain/purchase_order.go). supplier_id, like stock_items'
-- own, carries no FK for the same reason (no Supplier aggregate yet).
--
-- The "(status = 'recebido') = (received_at IS NOT NULL)" CHECK is the
-- database-level mirror of PurchaseOrder.MarkAsReceived's invariant: a
-- received order always has a non-null received_at, and a non-received one
-- never does -- there is no third state where the two disagree.
CREATE TABLE purchase_orders (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    supplier_id uuid NOT NULL, -- no FK: the Supplier aggregate does not exist in this phase.
    status text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    received_at timestamptz NULL,
    CONSTRAINT purchase_orders_status_check CHECK (status IN ('pendente', 'recebido', 'cancelado')),
    CONSTRAINT purchase_orders_status_received_at_check CHECK ((status = 'recebido') = (received_at IS NOT NULL)),
    CONSTRAINT purchase_orders_tenant_id_id_key UNIQUE (tenant_id, id)
);

ALTER TABLE purchase_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_orders FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON purchase_orders
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON purchase_orders TO mesaposta_app;

-- purchase_order_items: PurchaseOrderItem, a value object with no identity
-- of its own outside the PurchaseOrder that owns it (see
-- domain/purchase_order.go) -- hence the surrogate "id" primary key
-- (needed for the row to have one at all) is never read back by
-- application code, only ever generated by the database default.
-- UNIQUE (tenant_id, purchase_order_id, stock_item_id) is the physical
-- mirror of NewPurchaseOrder's own in-memory invariant that no two items on
-- the same order may reference the same stock item.
--
-- Like stock_movements below, this table is never updated after insert
-- (PurchaseOrder.Items() never changes once the order is created -- neither
-- MarkAsReceived nor Cancel touches it), so it is granted SELECT and INSERT
-- only.
CREATE TABLE purchase_order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    purchase_order_id uuid NOT NULL,
    stock_item_id uuid NOT NULL,
    quantity numeric(14,3) NOT NULL CHECK (quantity > 0),
    cost_per_unit numeric(14,2) NOT NULL CHECK (cost_per_unit >= 0),
    CONSTRAINT purchase_order_items_purchase_order_fk
        FOREIGN KEY (tenant_id, purchase_order_id) REFERENCES purchase_orders (tenant_id, id),
    CONSTRAINT purchase_order_items_stock_item_fk
        FOREIGN KEY (tenant_id, stock_item_id) REFERENCES stock_items (tenant_id, id),
    CONSTRAINT purchase_order_items_tenant_order_item_key UNIQUE (tenant_id, purchase_order_id, stock_item_id)
);

ALTER TABLE purchase_order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_order_items FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON purchase_order_items
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON purchase_order_items TO mesaposta_app;

-- stock_movements: an append-only log of every change to a StockItem's
-- quantity on hand (see domain/stock_movement.go's own doc comment: "There
-- is no setter and no pointer-receiver method anywhere in this file").
-- GRANT SELECT, INSERT only -- deliberately omitting UPDATE and DELETE --
-- is this table's own enforcement of that same immutability, at the one
-- layer application code cannot talk its way around: even a bug (or a
-- future engineer who "just this once" adds an UpdateMovement method) can
-- never issue an UPDATE or DELETE against this table as the mesaposta_app
-- role, because the privilege to do so was never granted in the first
-- place.
CREATE TABLE stock_movements (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    stock_item_id uuid NOT NULL,
    type text NOT NULL,
    quantity numeric(14,3) NOT NULL CHECK (quantity > 0),
    reason text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT stock_movements_type_check CHECK (type IN ('entrada', 'saida')),
    CONSTRAINT stock_movements_stock_item_fk
        FOREIGN KEY (tenant_id, stock_item_id) REFERENCES stock_items (tenant_id, id)
);

ALTER TABLE stock_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_movements FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON stock_movements
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON stock_movements TO mesaposta_app;

-- transactions: an append-only financial record -- revenue or expense --
-- posted to a tenant's books (see domain/transaction.go's own doc comment:
-- "immutable once created ... this phase exposes no setter"). Like
-- stock_movements above, granted SELECT and INSERT only. reference_id
-- carries no FK: it may point at a PurchaseOrder today, but is documented
-- (domain/transaction.go) as a generic "related entity" link, not
-- necessarily always a PurchaseOrder, so it is not tied to a single parent
-- table.
--
-- transactions_reference_compras_unique_idx is this module's acceptance
-- criterion 2 enforced as a physical, database-level backstop (layer 3 of
-- 3, behind the PurchaseOrder's own optimistic-concurrency version check
-- and the row lock FindByIDForUpdate takes): even if every application-
-- level guard were somehow bypassed, Postgres itself refuses a second
-- "compras" Transaction carrying a reference_id that already has one.
CREATE TABLE transactions (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    type text NOT NULL,
    category text NOT NULL,
    description text NOT NULL,
    amount numeric(14,2) NOT NULL CHECK (amount > 0),
    date timestamptz NOT NULL,
    reference_id uuid NULL,
    CONSTRAINT transactions_type_check CHECK (type IN ('receita', 'despesa')),
    CONSTRAINT transactions_category_check
        CHECK (category IN ('vendas', 'compras', 'salarios', 'aluguel', 'manutencao', 'marketing', 'outros'))
);

ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE transactions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON transactions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE UNIQUE INDEX transactions_reference_compras_unique_idx ON transactions (tenant_id, reference_id)
    WHERE reference_id IS NOT NULL AND category = 'compras';

GRANT SELECT, INSERT ON transactions TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS stock_movements;
DROP TABLE IF EXISTS purchase_order_items;
DROP TABLE IF EXISTS purchase_orders;
DROP TABLE IF EXISTS stock_items;
