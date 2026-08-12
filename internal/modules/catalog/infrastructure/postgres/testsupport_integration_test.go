//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
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

func mustDiscountPercent(t *testing.T, hundredths int64) domain.DiscountPercent {
	t.Helper()
	d, err := domain.NewDiscountPercent(hundredths)
	if err != nil {
		t.Fatalf("NewDiscountPercent(%d): %v", hundredths, err)
	}
	return d
}

// mustProduct builds and inserts a valid, ready-to-use Product for
// tenantID via the given repository, failing the test on any error.
func mustProduct(t *testing.T, ctx context.Context, repo *postgresadapter.ProductRepository, tenantID tenant.TenantID, name string) *domain.Product {
	t.Helper()
	product, err := domain.NewProduct(domain.NewID(), tenantID, name, "desc", mustMoney(t, 1500), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if err := repo.Insert(ctx, &product); err != nil {
		t.Fatalf("Insert(product): %v", err)
	}
	return &product
}

// mustRecipe builds and inserts a valid Recipe (with a single ingredient
// line) for tenantID via the given repository, failing the test on any
// error.
func mustRecipe(t *testing.T, ctx context.Context, repo *postgresadapter.RecipeRepository, tenantID tenant.TenantID, name string) *domain.Recipe {
	t.Helper()
	ingredient, err := domain.NewRecipeIngredient(domain.NewID(), mustQuantity(t, 200))
	if err != nil {
		t.Fatalf("NewRecipeIngredient: %v", err)
	}
	recipe, err := domain.NewRecipe(domain.NewID(), tenantID, name, "desc", []domain.RecipeIngredient{ingredient})
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}
	if err := repo.Insert(ctx, &recipe); err != nil {
		t.Fatalf("Insert(recipe): %v", err)
	}
	return &recipe
}

// buildTestDomainEvent builds a minimal, valid outbox.DomainEvent for
// tenantID/aggregateID, for tests that only care about a domain event's
// presence/tenant scoping, not its specific payload.
func buildTestDomainEvent(t *testing.T, tenantID tenant.TenantID, aggregateID string) (outbox.DomainEvent, error) {
	t.Helper()
	return outbox.NewDomainEvent(tenantID, "product.created", aggregateID, time.Now().UTC(), map[string]any{"test": true})
}
