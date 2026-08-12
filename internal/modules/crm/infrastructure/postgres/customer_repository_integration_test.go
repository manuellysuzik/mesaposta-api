//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestCustomerRepository_InsertAndFindByID_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCustomerRepository(scope)

	email := "maria@example.com"
	phone := "+5511999999999"
	customer, err := domain.NewCustomer(domain.NewID(), tenantID, "Maria", &email, &phone)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	if err := customer.CreditPoints(mustLoyaltyPoints(t, 42)); err != nil {
		t.Fatalf("CreditPoints: %v", err)
	}

	if err := repo.Insert(ctx, &customer); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := repo.FindByID(ctx, customer.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.ID() != customer.ID() {
		t.Errorf("found.ID() = %v, want %v", found.ID(), customer.ID())
	}
	if found.TenantID() != tenantID {
		t.Errorf("found.TenantID() = %v, want %v", found.TenantID(), tenantID)
	}
	if found.Name() != customer.Name() {
		t.Errorf("found.Name() = %q, want %q", found.Name(), customer.Name())
	}
	if found.Email() == nil || *found.Email() != email {
		t.Errorf("found.Email() = %v, want %v", found.Email(), email)
	}
	if found.Phone() == nil || *found.Phone() != phone {
		t.Errorf("found.Phone() = %v, want %v", found.Phone(), phone)
	}
	if found.LoyaltyPoints() != customer.LoyaltyPoints() {
		t.Errorf("found.LoyaltyPoints() = %v, want %v", found.LoyaltyPoints(), customer.LoyaltyPoints())
	}
	if found.Version() != customer.Version() {
		t.Errorf("found.Version() = %d, want %d", found.Version(), customer.Version())
	}
}

func TestCustomerRepository_FindByID_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCustomerRepository(scope)

	_, err = repo.FindByID(ctx, domain.NewID())
	if err == nil {
		t.Fatal("FindByID(unknown id): want an error, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestCustomerRepository_FindByID_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "30000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "30000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewCustomerRepository(scopeA)
	customer := mustCustomer(t, ctxA, repoA, tenantA, "Cliente A")
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewCustomerRepository(scopeB)

	_, err = repoB.FindByID(ctxB, customer.ID())
	if err == nil {
		t.Fatal("FindByID: tenant B found tenant A's customer, want it hidden by RLS")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestCustomerRepository_UpdateWithVersion_CrossTenant_AffectsZeroRows(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "30000000-0000-0000-0000-000000000005")
	tenantB := mustTenantID(t, "30000000-0000-0000-0000-000000000006")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	repoA := postgresadapter.NewCustomerRepository(scopeA)
	customer := mustCustomer(t, ctxA, repoA, tenantA, "Cliente A")
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewCustomerRepository(scopeB)

	mutated := *customer
	if err := mutated.CreditPoints(mustLoyaltyPoints(t, 10)); err != nil {
		t.Fatalf("CreditPoints: %v", err)
	}
	err = repoB.UpdateWithVersion(ctxB, &mutated, customer.Version())
	if err == nil {
		t.Fatal("UpdateWithVersion: tenant B updated tenant A's customer, want it rejected")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict (RowsAffected=0)", errs.KindOf(err))
	}
}

// TestCustomerRepository_UpdateWithVersion_StaleVersion_ReturnsConflict is
// the direct, single-transaction proof of this repository's optimistic
// concurrency contract: presenting an already-superseded expectedVersion
// must be rejected, not silently applied.
func TestCustomerRepository_UpdateWithVersion_StaleVersion_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000007")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCustomerRepository(scope)
	customer := mustCustomer(t, ctx, repo, tenantID, "Cliente Versao")

	mutated := *customer
	if err := mutated.CreditPoints(mustLoyaltyPoints(t, 10)); err != nil {
		t.Fatalf("CreditPoints: %v", err)
	}
	staleVersion := customer.Version() + 999 // deliberately wrong

	err = repo.UpdateWithVersion(ctx, &mutated, staleVersion)
	if err == nil {
		t.Fatal("UpdateWithVersion(stale version): want an error, got nil")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

// TestCustomerRepository_Insert_DuplicateEmailSameTenant_ReturnsConflict
// proves the database's own per-tenant unique constraint on email (see
// 0006_crm.sql's customers_tenant_email_unique_idx) is surfaced as an
// *errs.Error of Kind errs.Conflict, not a raw pgconn error.
func TestCustomerRepository_Insert_DuplicateEmailSameTenant_ReturnsConflict(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-000000000008")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewCustomerRepository(scope)

	email := "duplicado@example.com"
	first, err := domain.NewCustomer(domain.NewID(), tenantID, "Primeiro", &email, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	if err := repo.Insert(ctx, &first); err != nil {
		t.Fatalf("Insert(first): %v", err)
	}

	second, err := domain.NewCustomer(domain.NewID(), tenantID, "Segundo", &email, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	err = repo.Insert(ctx, &second)
	if err == nil {
		t.Fatal("Insert(duplicate email, same tenant): want an error, got nil")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

// TestCustomerRepository_Insert_SameEmailDifferentTenants_Succeeds proves
// email uniqueness is scoped PER TENANT, not global: two different tenants
// may each have a customer with the same email address.
func TestCustomerRepository_Insert_SameEmailDifferentTenants_Succeeds(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "30000000-0000-0000-0000-000000000009")
	tenantB := mustTenantID(t, "30000000-0000-0000-0000-00000000000a")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	email := "compartilhado@example.com"

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	defer scopeA.Rollback(ctxA)
	repoA := postgresadapter.NewCustomerRepository(scopeA)
	customerA, err := domain.NewCustomer(domain.NewID(), tenantA, "Cliente A", &email, nil)
	if err != nil {
		t.Fatalf("NewCustomer (A): %v", err)
	}
	if err := repoA.Insert(ctxA, &customerA); err != nil {
		t.Fatalf("Insert (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	repoB := postgresadapter.NewCustomerRepository(scopeB)
	customerB, err := domain.NewCustomer(domain.NewID(), tenantB, "Cliente B", &email, nil)
	if err != nil {
		t.Fatalf("NewCustomer (B): %v", err)
	}
	if err := repoB.Insert(ctxB, &customerB); err != nil {
		t.Fatalf("Insert (B): want success (email uniqueness is per-tenant), got error: %v", err)
	}
}

func TestLoyaltyEntries_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "30000000-0000-0000-0000-00000000000b")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	customerRepo := postgresadapter.NewCustomerRepository(setupScope)
	customer := mustCustomer(t, ctx, customerRepo, tenantID, "Cliente Ledger")

	entry, err := domain.NewLoyaltyEntry(domain.NewID(), tenantID, customer.ID(), domain.LoyaltyEntryTypeCredito, mustLoyaltyPoints(t, 10), "bonus")
	if err != nil {
		t.Fatalf("NewLoyaltyEntry: %v", err)
	}
	entryRepo := postgresadapter.NewLoyaltyEntryRepository(setupScope)
	if err := entryRepo.Insert(ctx, entry); err != nil {
		t.Fatalf("Insert(entry): %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE loyalty_entries SET reason = 'tampered' WHERE id = $1", entry.ID().String()); err == nil {
		t.Fatal("UPDATE loyalty_entries: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
	if _, err := scope.Exec(ctx, "DELETE FROM loyalty_entries WHERE id = $1", entry.ID().String()); err == nil {
		t.Fatal("DELETE FROM loyalty_entries: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}
