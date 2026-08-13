//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// openTestPool starts an ephemeral, fully-migrated Postgres container (via
// db.TestPostgres, the same helper every other module's own integration
// suite already reuses) and returns a Pool connected as the restricted
// mesaposta_app role.
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

// mustTable builds and inserts a valid, ready-to-use Table via repo,
// failing the test on any error.
func mustTable(t *testing.T, ctx context.Context, repo *postgresadapter.TableRepository, tenantID tenant.TenantID, code string) *domain.Table {
	t.Helper()
	table, err := domain.NewTable(domain.NewID(), tenantID, code, "Mesa "+code)
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	if err := repo.Insert(ctx, &table); err != nil {
		t.Fatalf("Insert(table): %v", err)
	}
	return &table
}

func mustOrderQuantity(t *testing.T, count int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(count)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", count, err)
	}
	return q
}

func mustOrderMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

// mustOrder builds and inserts a valid, single-item Order against tableID
// via repo, failing the test on any error.
func mustOrder(t *testing.T, ctx context.Context, repo *postgresadapter.OrderRepository, tenantID tenant.TenantID, tableID domain.ID) *domain.Order {
	t.Helper()
	item, err := domain.NewOrderItem("catalog-product-1", mustOrderQuantity(t, 2), mustOrderMoney(t, 1500))
	if err != nil {
		t.Fatalf("NewOrderItem: %v", err)
	}
	order, err := domain.NewOrder(domain.NewID(), tenantID, tableID, nil, nil, []domain.OrderItem{item})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Insert(ctx, &order); err != nil {
		t.Fatalf("Insert(order): %v", err)
	}
	return &order
}
