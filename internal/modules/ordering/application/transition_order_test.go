package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustFakeQuantity(t *testing.T, count int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(count)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", count, err)
	}
	return q
}

func mustFakeMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

func mustFakeOrder(t *testing.T, tableID domain.ID) *domain.Order {
	t.Helper()
	item, err := domain.NewOrderItem("p1", mustFakeQuantity(t, 2), mustFakeMoney(t, 1000))
	if err != nil {
		t.Fatalf("NewOrderItem: %v", err)
	}
	order, err := domain.NewOrder(domain.NewID(), mustFakeUOWTenantID(t), tableID, nil, nil, []domain.OrderItem{item})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return &order
}

func TestConfirmOrder_Valid_AdvancesStatusAndPersists(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	got, err := application.ConfirmOrder(context.Background(), uow, order.ID())
	if err != nil {
		t.Fatalf("ConfirmOrder: unexpected error: %v", err)
	}
	if got.Status() != domain.OrderStatusConfirmado {
		t.Errorf("Status() = %q, want %q", got.Status(), domain.OrderStatusConfirmado)
	}
	persisted := uow.snapshot().orders[order.ID()]
	if persisted.Status() != domain.OrderStatusConfirmado {
		t.Errorf("persisted Status() = %q, want %q", persisted.Status(), domain.OrderStatusConfirmado)
	}
}

func TestConfirmOrder_NotFound_ReturnsNotFound(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())

	_, err := application.ConfirmOrder(context.Background(), uow, domain.NewID())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

// TestStartPreparingOrder_WithoutConfirm_ReturnsConflictAndLeavesUntouched
// proves invalid transitions surface as errs.Conflict at the application
// layer AND leave the persisted order completely untouched -- the mutate
// callback fails before UpdateWithVersion is ever reached.
func TestStartPreparingOrder_WithoutConfirm_ReturnsConflictAndLeavesUntouched(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	uow.errors.orderUpdateWithVersion = errUnexpectedPersistCall

	_, err := application.StartPreparingOrder(context.Background(), uow, order.ID())
	if !errs.IsKind(err, errs.Conflict) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
	persisted := uow.snapshot().orders[order.ID()]
	if persisted.Status() != domain.OrderStatusPendenteConfirmacao {
		t.Errorf("persisted Status() = %q, want unchanged %q", persisted.Status(), domain.OrderStatusPendenteConfirmacao)
	}
}

func TestOrderLifecycle_ConfirmPreparePronoDeliver(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	ctx := context.Background()

	if _, err := application.ConfirmOrder(ctx, uow, order.ID()); err != nil {
		t.Fatalf("ConfirmOrder: %v", err)
	}
	if _, err := application.StartPreparingOrder(ctx, uow, order.ID()); err != nil {
		t.Fatalf("StartPreparingOrder: %v", err)
	}
	if _, err := application.MarkOrderReady(ctx, uow, order.ID()); err != nil {
		t.Fatalf("MarkOrderReady: %v", err)
	}
	got, err := application.MarkOrderDelivered(ctx, uow, order.ID())
	if err != nil {
		t.Fatalf("MarkOrderDelivered: %v", err)
	}
	if got.Status() != domain.OrderStatusEntregue {
		t.Errorf("Status() = %q, want %q", got.Status(), domain.OrderStatusEntregue)
	}
}

func TestCancelOrder_FromPendente_Succeeds(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	got, err := application.CancelOrder(context.Background(), uow, order.ID())
	if err != nil {
		t.Fatalf("CancelOrder: unexpected error: %v", err)
	}
	if got.Status() != domain.OrderStatusCancelado {
		t.Errorf("Status() = %q, want %q", got.Status(), domain.OrderStatusCancelado)
	}
}

func TestGetOrder_Found(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	got, err := application.GetOrder(context.Background(), uow, order.ID())
	if err != nil {
		t.Fatalf("GetOrder: unexpected error: %v", err)
	}
	if got.ID() != order.ID() {
		t.Errorf("got.ID() = %v, want %v", got.ID(), order.ID())
	}
}

func TestListOrders_ReturnsEveryOrder(t *testing.T) {
	table := mustFakeTable(t)
	order := mustFakeOrder(t, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	got, err := application.ListOrders(context.Background(), uow)
	if err != nil {
		t.Fatalf("ListOrders: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}
