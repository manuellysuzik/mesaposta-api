package application

import (
	"context"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
)

// auditActionOrderStatusChanged and auditEntityOrder are this file's audit
// action/entity identifiers -- one generic action shared by all five
// transitions below (rather than five separate ones), since they are all
// literally the same shape of event (an Order's status field changed) and
// the before/after status recorded in the entry itself is what actually
// distinguishes them; see this file's own doc comment on mutateOrder for
// why the transitions share one code path in the first place. Constants
// rather than inline literals for the same reason
// internal/modules/operations/application.ReceivePurchaseOrder's own audit
// constants are: the value of an audit trail depends on these identifiers
// being STABLE across time.
const (
	auditActionOrderStatusChanged = "order.status_changed"
	auditEntityOrder              = "order"
)

// mutateOrder loads the Order identified by id, applies mutate to it, and
// -- only if mutate succeeds -- persists the result via UpdateWithVersion,
// enforcing optimistic concurrency against the version the order was
// loaded at, then records an audit entry for the status transition. Every
// one of this file's exported functions (ConfirmOrder, StartPreparingOrder,
// MarkOrderReady, MarkOrderDelivered, CancelOrder) is this exact shape with
// a different one-line mutate function, so it is factored out once here
// rather than duplicated five times (DRY) -- including the audit write,
// which is why every transition gets one without repeating it five times.
//
// actorID identifies who requested the transition, for the audit trail:
// best-effort ("" when unknown, never a placeholder), sourced by the HTTP
// layer from the authenticated access token's "sub" claim (see
// internal/platform/auth.ActorIDFrom). See docs/adr/0006-audit-log.md.
func mutateOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string, mutate func(*domain.Order) error) (domain.Order, error) {
	var order domain.Order
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Orders().FindByID(ctx, id)
		if err != nil {
			return err
		}
		expectedVersion := found.Version()
		statusBefore := found.Status()

		if err := mutate(found); err != nil {
			return err
		}

		if err := r.Orders().UpdateWithVersion(ctx, found, expectedVersion); err != nil {
			return err
		}

		// The audit entry goes last, after the write it describes, on the
		// same principle operations.ReceivePurchaseOrder's own audit write
		// follows: because it runs inside this same Do call (the same
		// transaction), a failure anywhere -- including here -- rolls back
		// the transition AND its trail together.
		entry, err := audit.NewEntry(
			found.TenantID(),
			actorID,
			auditActionOrderStatusChanged,
			auditEntityOrder,
			found.ID().String(),
			time.Now().UTC(),
			map[string]any{"status": string(statusBefore)},
			map[string]any{"status": string(found.Status())},
		)
		if err != nil {
			return err
		}
		if err := r.Audit().Record(ctx, entry); err != nil {
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
func ConfirmOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, actorID, (*domain.Order).Confirm)
}

// StartPreparingOrder transitions an order from OrderStatusConfirmado to
// OrderStatusEmPreparo.
func StartPreparingOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, actorID, (*domain.Order).StartPreparing)
}

// MarkOrderReady transitions an order from OrderStatusEmPreparo to
// OrderStatusPronto.
func MarkOrderReady(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, actorID, (*domain.Order).MarkReady)
}

// MarkOrderDelivered transitions an order from OrderStatusPronto to
// OrderStatusEntregue.
func MarkOrderDelivered(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, actorID, (*domain.Order).MarkDelivered)
}

// CancelOrder transitions an order to OrderStatusCancelado from any
// non-terminal status.
func CancelOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID, actorID string) (domain.Order, error) {
	return mutateOrder(ctx, uow, id, actorID, (*domain.Order).Cancel)
}
