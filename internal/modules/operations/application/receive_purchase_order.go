package application

import (
	"context"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
)

// Audit action/entity identifiers this use case records. They are
// constants rather than inline literals because the value of an audit
// trail depends entirely on the identifiers being STABLE: a query written
// today against "purchase_order.received" must keep finding the same
// events years from now, and a typo in a literal would silently split one
// action into two un-joinable halves.
const (
	auditActionPurchaseOrderReceived = "purchase_order.received"
	auditEntityPurchaseOrder         = "purchase_order"
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
//
// actorID identifies who requested the receipt, for the audit trail. It is
// best-effort by design: the HTTP layer fills it from the authenticated
// access token's "sub" claim (see internal/platform/auth.ClaimsFromContext)
// and passes the empty string when there is no identifiable human actor --
// a scheduled job, an internal replay. An empty actorID is recorded as SQL
// NULL, never as a placeholder, so "the system did this" stays distinct
// from any real actor id. See docs/adr/0006-audit-log.md for why this stays
// a plain string until an identity module exists.
func ReceivePurchaseOrder(ctx context.Context, uow domain.UnitOfWork, orderID domain.ID, now time.Time, actorID string) error {
	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		order, err := r.PurchaseOrders().FindByIDForUpdate(ctx, orderID)
		if err != nil {
			return err
		}
		// Snapshot BEFORE domain.ReceivePurchaseOrder mutates order in
		// place: after that call, order.Status()/Version() already report
		// the post-receipt state, so reading them later would record a
		// "before" identical to the "after" -- an audit entry that shows
		// no change at all.
		orderVersionBeforeReceive := order.Version()
		orderStatusBeforeReceive := order.Status()

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

		// The audit entry goes last, after every other write, on the same
		// principle the outbox publish above follows: it is one more
		// effect to persist atomically, and ordering it last keeps the
		// early-conflict-detection property the PurchaseOrder's own
		// UpdateWithVersion-first ordering exists for. Because it runs
		// inside the same Do (and therefore the same transaction), a
		// failure anywhere -- including here -- rolls back the mutation
		// AND its trail together: there is no state in which the order was
		// received but nothing recorded it, nor one in which the trail
		// claims a receipt that never committed.
		entry, err := audit.NewEntry(
			result.Order.TenantID(),
			actorID,
			auditActionPurchaseOrderReceived,
			auditEntityPurchaseOrder,
			result.Order.ID().String(),
			now,
			// Small, targeted snapshots -- not full aggregate dumps. The
			// trail must stay cheap enough to always write, and must not
			// become a second, drifting copy of the aggregate; what an
			// investigator actually needs here is the state transition and
			// the version it happened at.
			map[string]any{
				"status":  string(orderStatusBeforeReceive),
				"version": orderVersionBeforeReceive,
			},
			map[string]any{
				"status":  string(result.Order.Status()),
				"version": result.Order.Version(),
			},
		)
		if err != nil {
			return err
		}
		if err := r.Audit().Record(ctx, entry); err != nil {
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
