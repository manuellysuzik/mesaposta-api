//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// openTestPool starts an ephemeral, fully-migrated Postgres container (via
// db.TestPostgres, the same helper internal/platform/outbox/postgres,
// internal/platform/auth/refresh and
// internal/modules/operations/infrastructure/postgres's own integration
// suites already reuse) and returns a Pool connected as the restricted
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

func mustLoyaltyPoints(t *testing.T, points int64) domain.LoyaltyPoints {
	t.Helper()
	p, err := domain.NewLoyaltyPoints(points)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(%d): %v", points, err)
	}
	return p
}

// mustCustomer builds and inserts a valid, ready-to-use Customer for
// tenantID via the given repository, failing the test on any error.
func mustCustomer(t *testing.T, ctx context.Context, repo *postgresadapter.CustomerRepository, tenantID tenant.TenantID, name string) *domain.Customer {
	t.Helper()
	customer, err := domain.NewCustomer(domain.NewID(), tenantID, name, nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	if err := repo.Insert(ctx, &customer); err != nil {
		t.Fatalf("Insert(customer): %v", err)
	}
	return &customer
}

// buildTestDomainEvent builds a minimal, valid outbox.DomainEvent for
// tenantID/aggregateID, for tests that only care about a domain event's
// presence/tenant scoping, not its specific payload.
func buildTestDomainEvent(t *testing.T, tenantID tenant.TenantID, aggregateID string) (outbox.DomainEvent, error) {
	t.Helper()
	return outbox.NewDomainEvent(tenantID, "customer.created", aggregateID, time.Now().UTC(), map[string]any{"test": true})
}
