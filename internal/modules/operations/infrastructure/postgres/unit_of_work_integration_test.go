//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestUnitOfWork_Do_SuccessfulFn_CommitsEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "10000000-0000-0000-0000-000000000001")
	ctx := tenant.With(context.Background(), tenantID)

	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Farinha de trigo", domain.StockUnitKg,
		mustQuantity(t, 0), mustQuantity(t, 0), mustMoney(t, 500), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.StockItems().Insert(ctx, &item)
	})
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.StockItems().FindByID(ctx, item.ID())
		if err != nil {
			return err
		}
		if found.ID() != item.ID() {
			t.Errorf("found.ID() = %v, want %v", found.ID(), item.ID())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do (verify): unexpected error: %v", err)
	}
}

// TestUnitOfWork_Do_FailingFn_RollsBackEverything is T5.2's headline RED
// test: a Do call whose fn writes to several different tables and then
// returns an error must leave ZERO rows behind, across every one of the 5
// operations tables -- not just the specific write that happened to fail.
func TestUnitOfWork_Do_FailingFn_RollsBackEverything(t *testing.T) {
	pool := openTestPool(t)
	uow := postgresadapter.NewUnitOfWork(pool)

	tenantID := mustTenantID(t, "10000000-0000-0000-0000-000000000002")
	ctx := tenant.With(context.Background(), tenantID)

	sentinel := errors.New("boom")

	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Acucar", domain.StockUnitKg,
		mustQuantity(t, 0), mustQuantity(t, 0), mustMoney(t, 300), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.StockItems().Insert(ctx, &item); err != nil {
			return err
		}

		if err := item.Receive(mustQuantity(t, 1000)); err != nil {
			return err
		}
		if err := r.StockItems().UpdateWithVersion(ctx, &item, 1); err != nil {
			return err
		}

		movement, err := domain.NewStockMovement(
			domain.NewID(), tenantID, item.ID(), domain.StockMovementTypeEntrada, mustQuantity(t, 1000), "test",
		)
		if err != nil {
			return err
		}
		if err := r.StockMovements().Insert(ctx, movement); err != nil {
			return err
		}

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}

	// Nothing must have been persisted: not the stock item, not its
	// movement.
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		_, err := r.StockItems().FindByID(ctx, item.ID())
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
// authenticated tenant -- db.Pool.Scope checks tenant.From(ctx) BEFORE
// calling pool.Begin, so this is really exercising that guarantee through
// UnitOfWork.Do's own surface.
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

	tenantID := mustTenantID(t, "10000000-0000-0000-0000-000000000003")
	ctx := tenant.With(context.Background(), tenantID)

	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Sal", domain.StockUnitKg,
		mustQuantity(t, 0), mustQuantity(t, 0), mustMoney(t, 100), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}

	event, err := buildTestDomainEvent(t, tenantID, item.ID().String())
	if err != nil {
		t.Fatalf("buildTestDomainEvent: %v", err)
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.StockItems().Insert(ctx, &item); err != nil {
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
	if err := scope.QueryRow(ctx, "SELECT count(*) FROM domain_events WHERE aggregate_id = $1", item.ID().String()).Scan(&count); err != nil {
		t.Fatalf("counting domain_events: %v", err)
	}
	if count != 1 {
		t.Errorf("domain_events count = %d, want 1", count)
	}
}
