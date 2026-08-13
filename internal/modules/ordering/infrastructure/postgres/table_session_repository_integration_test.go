//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestTableSessionRepository_InsertAndFindByTokenHash_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	tableRepo := postgresadapter.NewTableRepository(scope)
	sessionRepo := postgresadapter.NewTableSessionRepository(scope)

	table := mustTable(t, ctx, tableRepo, tenantID, "M01")
	session, plaintext, err := domain.NewTableSession(tenantID, table.ID())
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}
	if err := sessionRepo.Insert(ctx, session); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := sessionRepo.FindByTokenHash(ctx, domain.HashSessionToken(plaintext))
	if err != nil {
		t.Fatalf("FindByTokenHash: %v", err)
	}
	if found.TableID() != table.ID() {
		t.Errorf("found.TableID() = %v, want %v", found.TableID(), table.ID())
	}
	if found.TokenHash() != session.TokenHash() {
		t.Error("found.TokenHash() does not match the inserted session's hash")
	}
}

func TestTableSessionRepository_FindByTokenHash_Unknown_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantID := mustTenantID(t, "60000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)
	sessionRepo := postgresadapter.NewTableSessionRepository(scope)

	_, err = sessionRepo.FindByTokenHash(ctx, domain.HashSessionToken("never-issued"))
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestTableSessionRepository_FindByTokenHash_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	tenantA := mustTenantID(t, "60000000-0000-0000-0000-000000000003")
	tenantB := mustTenantID(t, "60000000-0000-0000-0000-000000000004")
	ctxA := tenant.With(context.Background(), tenantA)
	ctxB := tenant.With(context.Background(), tenantB)

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (A): %v", err)
	}
	tableRepoA := postgresadapter.NewTableRepository(scopeA)
	sessionRepoA := postgresadapter.NewTableSessionRepository(scopeA)
	tableA := mustTable(t, ctxA, tableRepoA, tenantA, "M03")
	sessionA, plaintextA, err := domain.NewTableSession(tenantA, tableA.ID())
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}
	if err := sessionRepoA.Insert(ctxA, sessionA); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := scopeA.Commit(ctxA); err != nil {
		t.Fatalf("commit (A): %v", err)
	}

	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	sessionRepoB := postgresadapter.NewTableSessionRepository(scopeB)

	_, err = sessionRepoB.FindByTokenHash(ctxB, domain.HashSessionToken(plaintextA))
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound (RLS must hide tenant A's session from tenant B)", errs.KindOf(err))
	}
}
