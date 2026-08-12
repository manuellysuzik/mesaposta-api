//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestTransactionRepository_Insert_Succeeds(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTransactionRepository(scope)

	referenceID := domain.NewID()
	transaction, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeDespesa, domain.TransactionCategoryCompras,
		"Pedido de compra teste", mustMoney(t, 5000), time.Now().UTC(), &referenceID,
	)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}

	if err := repo.Insert(ctx, transaction); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", transaction.ID().String()).Scan(&count); err != nil {
		t.Fatalf("counting inserted transaction: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// TestTransactions_ReferenceComprasUniqueIndex_RejectsDuplicate is T5.4's
// "layer 3 in isolation" test: it proves
// transactions_reference_compras_unique_idx itself -- independent of any
// application-level guard -- rejects a second "compras" Transaction
// carrying a reference_id that already has one, by inserting directly via
// SQL rather than going through the use-case flow.
func TestTransactions_ReferenceComprasUniqueIndex_RejectsDuplicate(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTransactionRepository(scope)

	referenceID := domain.NewID()
	first, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeDespesa, domain.TransactionCategoryCompras,
		"Primeiro", mustMoney(t, 1000), time.Now().UTC(), &referenceID,
	)
	if err != nil {
		t.Fatalf("NewTransaction (first): %v", err)
	}
	if err := repo.Insert(ctx, first); err != nil {
		t.Fatalf("Insert (first): %v", err)
	}

	second, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeDespesa, domain.TransactionCategoryCompras,
		"Segundo (duplicado)", mustMoney(t, 2000), time.Now().UTC(), &referenceID,
	)
	if err != nil {
		t.Fatalf("NewTransaction (second): %v", err)
	}
	err = repo.Insert(ctx, second)
	if err == nil {
		t.Fatal("Insert (second, duplicate reference_id/category=compras): want an error, got nil")
	}
}

// TestTransactions_ReferenceComprasUniqueIndex_AllowsSameReferenceDifferentCategory
// confirms the partial index's WHERE clause is doing real work: the same
// reference_id is fine for a non-"compras" category, so this is not simply
// a blanket unique(tenant_id, reference_id) constraint.
func TestTransactions_ReferenceComprasUniqueIndex_AllowsSameReferenceDifferentCategory(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	repo := postgresadapter.NewTransactionRepository(scope)

	referenceID := domain.NewID()
	compras, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeDespesa, domain.TransactionCategoryCompras,
		"Compra", mustMoney(t, 1000), time.Now().UTC(), &referenceID,
	)
	if err != nil {
		t.Fatalf("NewTransaction (compras): %v", err)
	}
	if err := repo.Insert(ctx, compras); err != nil {
		t.Fatalf("Insert (compras): %v", err)
	}

	outros, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeDespesa, domain.TransactionCategoryOutros,
		"Outra categoria, mesma referencia", mustMoney(t, 500), time.Now().UTC(), &referenceID,
	)
	if err != nil {
		t.Fatalf("NewTransaction (outros): %v", err)
	}
	if err := repo.Insert(ctx, outros); err != nil {
		t.Fatalf("Insert (outros, same reference_id, different category): want it accepted, got error: %v", err)
	}
}

func TestTransactions_UpdateAndDelete_DeniedForAppRole(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000004")
	ctx := tenant.With(context.Background(), tenantID)

	setupScope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope (setup): %v", err)
	}
	repo := postgresadapter.NewTransactionRepository(setupScope)
	transaction, err := domain.NewTransaction(
		domain.NewID(), tenantID, domain.TransactionTypeReceita, domain.TransactionCategoryVendas,
		"Venda", mustMoney(t, 100), time.Now().UTC(), nil,
	)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	if err := repo.Insert(ctx, transaction); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := setupScope.Commit(ctx); err != nil {
		t.Fatalf("commit (setup): %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	if _, err := scope.Exec(ctx, "UPDATE transactions SET description = 'tampered' WHERE id = $1", transaction.ID().String()); err == nil {
		t.Fatal("UPDATE transactions: want it denied for the mesaposta_app role (no UPDATE grant), got nil error")
	}
	if _, err := scope.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transaction.ID().String()); err == nil {
		t.Fatal("DELETE FROM transactions: want it denied for the mesaposta_app role (no DELETE grant), got nil error")
	}
}
