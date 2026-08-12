package application

import (
	"context"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// ReceivePurchaseOrder is the application-layer use case that receives a
// pending PurchaseOrder identified by orderID. It loads the order (with a
// row lock, via FindByIDForUpdate) and every StockItem it references,
// delegates the actual state transition to the pure domain service
// domain.ReceivePurchaseOrder, and -- only if that succeeds -- persists
// everything it produced (the updated PurchaseOrder, every mutated
// StockItem, the new StockMovements, the new Transaction, and every raised
// domain event) atomically, within a single UnitOfWork.Do call.
//
// Persistence order within Do is significant and intentional: the
// PurchaseOrder's own UpdateWithVersion always happens first, before any
// StockItem/StockMovement/Transaction write, so that a concurrent conflict
// on the order itself (an *errs.Error of Kind errs.Conflict, in a real
// Postgres-backed UnitOfWork per domain.PurchaseOrderRepository's doc
// comment) is detected as early as possible, before any other write in
// this unit of work is even attempted.
//
// If domain.ReceivePurchaseOrder itself fails (the order was already
// received or cancelled, or a stock item it references was not found), or
// if any downstream repository write fails, Do rolls back in full and
// ReceivePurchaseOrder returns that error with its original errs.Kind
// preserved -- nothing is persisted. Note that on the
// already-received/cancelled failure path, the *domain.PurchaseOrder
// loaded by FindByIDForUpdate above is left mutated in memory (see
// domain/receiving.go's doc comment) -- but since that pointer only ever
// lives inside this single, rolled-back Do call, and is never reused
// across calls, that in-memory mutation never escapes to a persisted
// state or to a subsequent call.
func ReceivePurchaseOrder(ctx context.Context, uow domain.UnitOfWork, orderID domain.ID, now time.Time) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		order, err := r.PurchaseOrders().FindByIDForUpdate(ctx, orderID)
		if err != nil {
			return err
		}
		orderVersionBeforeReceive := order.Version()

		stockItemIDs := purchaseOrderStockItemIDs(order)
		stockItems, err := r.StockItems().FindManyByIDs(ctx, stockItemIDs)
		if err != nil {
			return err
		}
		stockItemVersionsBeforeReceive := stockItemVersionsOf(stockItems)

		result, err := domain.ReceivePurchaseOrder(order, stockItems, now)
		if err != nil {
			return err
		}

		if err := r.PurchaseOrders().UpdateWithVersion(ctx, result.Order, orderVersionBeforeReceive); err != nil {
			return err
		}

		for _, item := range result.StockItems {
			expectedVersion := stockItemVersionsBeforeReceive[item.ID()]
			if err := r.StockItems().UpdateWithVersion(ctx, item, expectedVersion); err != nil {
				return err
			}
		}

		for _, movement := range result.Movements {
			if err := r.StockMovements().Insert(ctx, movement); err != nil {
				return err
			}
		}

		if err := r.Transactions().Insert(ctx, result.Transaction); err != nil {
			return err
		}

		if err := r.Events().Publish(ctx, result.Events...); err != nil {
			return err
		}

		return nil
	})
}

// purchaseOrderStockItemIDs returns the IDs of every distinct StockItem
// order's line items reference.
func purchaseOrderStockItemIDs(order *domain.PurchaseOrder) []domain.ID {
	items := order.Items()
	ids := make([]domain.ID, len(items))
	for i, item := range items {
		ids[i] = item.StockItemID()
	}
	return ids
}

// stockItemVersionsOf snapshots each stock item's Version BEFORE any
// domain mutation is applied to it, so the caller can later pass the
// correct expectedVersion into StockItemRepository.UpdateWithVersion.
func stockItemVersionsOf(stockItems map[domain.ID]*domain.StockItem) map[domain.ID]int64 {
	versions := make(map[domain.ID]int64, len(stockItems))
	for id, item := range stockItems {
		versions[id] = item.Version()
	}
	return versions
}
