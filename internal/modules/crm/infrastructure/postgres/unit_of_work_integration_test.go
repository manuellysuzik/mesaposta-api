//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestUnitOfWork_Do_SuccessfulFn_CommitsEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	customer, err := domain.NewCustomer(domain.NewID(), tenantID, "Cliente UOW", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Customers().Insert(ctx, &customer)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Customers().FindByID(ctx, customer.ID())
		if err != nil {
			return err
		}
		if found.ID() != customer.ID() {
			t.Errorf("found.ID() = %v, want %v", found.ID(), customer.ID())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_FailingFn_RollsBackEverything proves that a Do call
// touching both customers and loyalty_entries, that then returns an error,
// leaves ZERO rows behind in either table -- this module's headline
// acceptance criterion (Customer balance + LoyaltyEntry persisted
// together, or not at all) proved directly against a real Postgres
// transaction.
func TestUnitOfWork_Do_FailingFn_RollsBackEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	sentinel := errors.New("boom")

	customer, err := domain.NewCustomer(domain.NewID(), tenantID, "Cliente Rollback", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Customers().Insert(ctx, &customer); err != nil {
			return err
		}

		if err := customer.CreditPoints(mustLoyaltyPoints(t, 100)); err != nil {
			return err
		}
		if err := r.Customers().UpdateWithVersion(ctx, &customer, 1); err != nil {
			return err
		}

		entry, err := domain.NewLoyaltyEntry(
			domain.NewID(), tenantID, customer.ID(), domain.LoyaltyEntryTypeCredito, mustLoyaltyPoints(t, 100), "test",
		)
		if err != nil {
			return err
		}
		if err := r.LoyaltyEntries().Insert(ctx, entry); err != nil {
			return err
		}

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}

	// Nothing must have been persisted: not the customer, not its ledger
	// entry.
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		_, err := r.Customers().FindByID(ctx, customer.ID())
		if !errs.IsKind(err, errs.NotFound) {
			t.Errorf("FindByID after rollback: errs.KindOf(err) = %v, want errs.NotFound (err=%v)", errs.KindOf(err), err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_NoTenantInContext_ReturnsUnauthenticatedWithoutTouchingDB
// proves Do never acquires a database connection at all when ctx carries no
// authenticated tenant.
func TestUnitOfWork_Do_NoTenantInContext_ReturnsUnauthenticatedWithoutTouchingDB(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	called := false
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("Do without an authenticated tenant: want an error, got nil")
	}
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Unauthenticated", errs.KindOf(err))
	}
	if called {
		t.Error("Do without an authenticated tenant invoked fn -- it must never reach fn at all")
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Errorf("AcquiredConns() = %d, want 0 (no connection should ever be acquired)", got)
	}
}

func TestUnitOfWork_Events_PublishesWithinSameTransaction(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "40000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	customer, err := domain.NewCustomer(domain.NewID(), tenantID, "Cliente Evento", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}

	event, err := buildTestDomainEvent(t, tenantID, customer.ID().String())
	if err != nil {
		t.Fatalf("buildTestDomainEvent: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Customers().Insert(ctx, &customer); err != nil {
			return err
		}
		return r.Events().Publish(ctx, event)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	var count int
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM domain_events WHERE aggregate_id = $1", customer.ID().String()).Scan(&count); err != nil {
		t.Fatalf("counting domain_events: %v", err)
	}
	if count != 1 {
		t.Errorf("domain_events count = %d, want 1", count)
	}
}
