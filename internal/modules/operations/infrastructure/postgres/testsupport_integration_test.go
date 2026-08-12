//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// openTestPool starts an ephemeral, fully-migrated Postgres container (via
// db.TestPostgres, the same helper internal/platform/outbox/postgres and
// internal/platform/auth/refresh's own integration suites already reuse)
// and returns a Pool connected as the restricted mesaposta_app role.
func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	_, appURL := db.TestPostgres(t)
	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mustTenantID parses s (a canonical UUID literal) into a tenant.TenantID,
// failing the test on any error.
func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

func mustQuantity(t *testing.T, thousandths int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(thousandths)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", thousandths, err)
	}
	return q
}

func mustMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

// mustStockItem builds and inserts a valid, ready-to-use StockItem for
// tenantID via the given repository, failing the test on any error.
func mustStockItem(t *testing.T, ctx context.Context, repo *postgresadapter.StockItemRepository, tenantID tenant.TenantID, quantity int64) *domain.StockItem {
	t.Helper()
	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Farinha de trigo", domain.StockUnitKg,
		mustQuantity(t, quantity), mustQuantity(t, 0), mustMoney(t, 500), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}
	if err := repo.Insert(ctx, &item); err != nil {
		t.Fatalf("Insert(stock item): %v", err)
	}
	return &item
}

// mustPurchaseOrder builds and inserts a valid, pending PurchaseOrder
// referencing stockItemID for tenantID via the given repository, failing
// the test on any error.
func mustPurchaseOrder(
	t *testing.T, ctx context.Context, repo *postgresadapter.PurchaseOrderRepository,
	tenantID tenant.TenantID, stockItemID domain.ID, quantity, costPerUnit int64,
) *domain.PurchaseOrder {
	t.Helper()
	item, err := domain.NewPurchaseOrderItem(stockItemID, mustQuantity(t, quantity), mustMoney(t, costPerUnit))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem: %v", err)
	}
	order, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, domain.NewID(), []domain.PurchaseOrderItem{item})
	if err != nil {
		t.Fatalf("NewPurchaseOrder: %v", err)
	}
	if err := repo.Insert(ctx, &order); err != nil {
		t.Fatalf("Insert(purchase order): %v", err)
	}
	return &order
}

// buildTestDomainEvent builds a minimal, valid outbox.DomainEvent for
// tenantID/aggregateID, for tests that only care about a domain event's
// presence/tenant scoping, not its specific payload.
func buildTestDomainEvent(t *testing.T, tenantID tenant.TenantID, aggregateID string) (outbox.DomainEvent, error) {
	t.Helper()
	return outbox.NewDomainEvent(tenantID, "stock_item.created", aggregateID, time.Now().UTC(), map[string]any{"test": true})
}
