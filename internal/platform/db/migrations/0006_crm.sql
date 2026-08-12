-- 0006_crm.sql
--
-- Backing tables for internal/modules/crm's Customer aggregate (which
-- nests its loyalty ledger -- LoyaltyEntry -- as a child entity; see
-- internal/modules/crm/domain/customer.go's own doc comment for why that
-- aggregate boundary was chosen deliberately) as persisted by
-- internal/modules/crm/infrastructure/postgres. Follows the same
-- ownership/RLS convention as 0001/0002/0003/0004: every table is owned by
-- whichever administrative role runs migrations (never mesaposta_app), RLS
-- is enabled AND forced, and every policy is named "tenant_isolation" with
-- the exact same predicate used since 0001_baseline.sql:
--
--     USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
--     WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid)
--
-- Cross-tenant foreign key: customers carries a UNIQUE (tenant_id, id)
-- constraint, and loyalty_entries' foreign key is composite --
-- FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id)
-- -- instead of a bare FOREIGN KEY (customer_id) REFERENCES customers (id).
-- A single-column FK would let a row reference a same-"id" row belonging to
-- a DIFFERENT tenant (ids are globally unique UUIDs, so nothing else would
-- stop that); the composite FK makes a cross-tenant reference physically
-- impossible to insert, not just something RLS happens to hide afterwards
-- -- see 0004_operations.sql's own comment for the identical rationale.
--
-- Enum CHECK constraint on loyalty_entries.type mirrors
-- internal/modules/crm/domain/enums.go's LoyaltyEntryType closed set
-- exactly -- a second, physical backstop behind the application-level
-- ParseLoyaltyEntryType validation that type already performs.
--
-- loyalty_points is a plain bigint (a whole-number count of points, never
-- a float -- see internal/modules/crm/domain.LoyaltyPoints' own doc
-- comment), unlike 0004_operations.sql's numeric(14,2)/numeric(14,3)
-- monetary/quantity columns: there is no fractional scale to convert
-- between here, so no "$n::numeric / 1000"-style conversion is needed on
-- the way in or out.

-- +goose Up

-- customers: a tenant's customer, together with their loyalty program
-- balance (see domain/customer.go). email/phone are both optional
-- (nullable) but, when present, unique PER TENANT -- not globally -- via a
-- partial unique index (WHERE ... IS NOT NULL) so that any number of
-- customers with no email/phone on file can coexist without colliding on
-- NULL = NULL (which two-value UNIQUE constraints in Postgres never treat
-- as equal anyway, but the explicit WHERE clause documents the intent).
CREATE TABLE customers (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    email text NULL,
    phone text NULL,
    loyalty_points bigint NOT NULL DEFAULT 0 CHECK (loyalty_points >= 0),
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT customers_tenant_id_id_key UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX customers_tenant_email_unique_idx ON customers (tenant_id, email)
    WHERE email IS NOT NULL;
CREATE UNIQUE INDEX customers_tenant_phone_unique_idx ON customers (tenant_id, phone)
    WHERE phone IS NOT NULL;

ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON customers
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON customers TO mesaposta_app;

-- loyalty_entries: an append-only log of every change to a Customer's
-- loyalty point balance -- a credit or a redemption (see
-- domain/loyalty_entry.go's own doc comment: "There is no setter and no
-- pointer-receiver method anywhere in this file"). GRANT SELECT, INSERT
-- only -- deliberately omitting UPDATE and DELETE -- is this table's own
-- enforcement of that same immutability at the one layer application code
-- cannot talk its way around, mirroring 0004_operations.sql's identical
-- treatment of stock_movements/transactions.
CREATE TABLE loyalty_entries (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    type text NOT NULL,
    points bigint NOT NULL CHECK (points > 0),
    reason text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT loyalty_entries_type_check CHECK (type IN ('credito', 'resgate')),
    CONSTRAINT loyalty_entries_customer_fk
        FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id)
);

ALTER TABLE loyalty_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE loyalty_entries FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON loyalty_entries
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON loyalty_entries TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS loyalty_entries;
DROP TABLE IF EXISTS customers;
