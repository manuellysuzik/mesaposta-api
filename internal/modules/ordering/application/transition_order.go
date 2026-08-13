package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// mutateOrder loads the Order identified by id, applies mutate to it, and
// -- only if mutate succeeds -- persists the result via UpdateWithVersion,
// enforcing optimistic concurrency against the version the order was
// loaded at. Every one of this file's exported functions (ConfirmOrder,
// StartPreparingOrder, MarkOrderReady, MarkOrderDelivered, CancelOrder) is
// this exact shape with a different one-line mutate function, so it is
// factored out once here rather than duplicated five times (DRY).
func mutateOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID, mutate func(*domain.Order) error) (domain.Order, error) {
	var order domain.Order
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Orders().FindByID(ctx, id)
		if err != nil {
			return err
		}
		expectedVersion := found.Version()

		if err := mutate(found); err != nil {
			return err
		}

		if err := r.Orders().UpdateWithVersion(ctx, found, expectedVersion); err != nil {
			return err
		}
		order = *found
		return nil
	})
	if err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

// ConfirmOrder transitions an order from OrderStatusPendenteConfirmacao to
// OrderStatusConfirmado -- the authenticated-staff action that lets a
// QR-code order actually enter the kitchen queue (see domain.Order's own
// doc comment: every public order is born PENDING and never skips this
// step).
func ConfirmOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, (*domain.Order).Confirm)
}

// StartPreparingOrder transitions an order from OrderStatusConfirmado to
// OrderStatusEmPreparo.
func StartPreparingOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, (*domain.Order).StartPreparing)
}

// MarkOrderReady transitions an order from OrderStatusEmPreparo to
// OrderStatusPronto.
func MarkOrderReady(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, (*domain.Order).MarkReady)
}

// MarkOrderDelivered transitions an order from OrderStatusPronto to
// OrderStatusEntregue.
func MarkOrderDelivered(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, (*domain.Order).MarkDelivered)
}

// CancelOrder transitions an order to OrderStatusCancelado from any
// non-terminal status.
func CancelOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, (*domain.Order).Cancel)
}
