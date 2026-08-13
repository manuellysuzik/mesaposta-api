//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestUnitOfWork_Do_SuccessfulFn_CommitsEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "70000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	table, err := domain.NewTable(domain.NewID(), tenantID, "M01", "Mesa 1")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Tables().Insert(ctx, &table)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Tables().FindByID(ctx, table.ID())
		if err != nil {
			return err
		}
		if found.ID() != table.ID() {
			t.Errorf("found.ID() = %v, want %v", found.ID(), table.ID())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_FailingFn_RollsBackEverything proves that a Do call
// touching both the table and an order placed against it, that then
// returns an error, leaves ZERO rows behind in either table.
func TestUnitOfWork_Do_FailingFn_RollsBackEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "70000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	sentinel := errors.New("boom")

	table, err := domain.NewTable(domain.NewID(), tenantID, "M02", "Mesa 2")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	var orderID domain.ID
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Tables().Insert(ctx, &table); err != nil {
			return err
		}

		item, err := domain.NewOrderItem("p1", mustOrderQuantity(t, 1), mustOrderMoney(t, 100))
		if err != nil {
			return err
		}
		order, err := domain.NewOrder(domain.NewID(), tenantID, table.ID(), nil, nil, []domain.OrderItem{item})
		if err != nil {
			return err
		}
		orderID = order.ID()
		if err := r.Orders().Insert(ctx, &order); err != nil {
			return err
		}

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if _, err := r.Tables().FindByID(ctx, table.ID()); !errs.IsKind(err, errs.NotFound) {
			t.Errorf("Tables().FindByID after rollback: errs.KindOf(err) = %v, want errs.NotFound (err=%v)", errs.KindOf(err), err)
		}
		if _, err := r.Orders().FindByID(ctx, orderID); !errs.IsKind(err, errs.NotFound) {
			t.Errorf("Orders().FindByID after rollback: errs.KindOf(err) = %v, want errs.NotFound (err=%v)", errs.KindOf(err), err)
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
