package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// PurchaseOrderRepository is the Postgres-backed implementation of
// domain.PurchaseOrderRepository, backed by the purchase_orders and
// purchase_order_items tables (see
// internal/platform/db/migrations/0004_operations.sql). As with
// StockItemRepository, every query relies on row-level security (the
// tenant_isolation policy on both tables) rather than an application-level
// "tenant_id = $n" predicate, and every rehydrated PurchaseOrder's
// TenantID is set from scope.TenantID(), never re-parsed from the row.
type PurchaseOrderRepository struct {
	scope *db.Scope
}

// compile-time check: *PurchaseOrderRepository must satisfy domain.PurchaseOrderRepository.
var _ domain.PurchaseOrderRepository = (*PurchaseOrderRepository)(nil)

// NewPurchaseOrderRepository builds a PurchaseOrderRepository bound to
// scope -- see StockItemRepository's NewStockItemRepository doc comment
// for why this constructor exists alongside UnitOfWork.Do/Repositories.
// PurchaseOrders().
func NewPurchaseOrderRepository(scope *db.Scope) *PurchaseOrderRepository {
	return &PurchaseOrderRepository{scope: scope}
}

// purchaseOrderNotFoundErr builds the *errs.Error a missing-row lookup
// returns.
func purchaseOrderNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "purchase_order_not_found", "purchase order not found", cause)
}

// purchaseOrderConflictErr builds the *errs.Error UpdateWithVersion returns
// when its UPDATE affects zero rows.
func purchaseOrderConflictErr() error {
	return errs.New(errs.Conflict, "purchase_order_version_conflict",
		"purchase order was concurrently modified", nil)
}

const selectPurchaseOrderColumns = `id, supplier_id, status, version, created_at, received_at`

// selectPurchaseOrderItemsSQL loads a purchase order's line items, ordered
// ascending by stock_item_id. That ordering is not load-bearing for
// correctness on its own -- domain.ReceivePurchaseOrder re-sorts
// order.Items() itself before mutating anything (see receiving.go's doc
// comment) -- but keeping the read path in the same, consistent order as
// every write path this module performs (StockItemRepository.
// FindManyByIDs' own ORDER BY id ASC, and the ascending-by-ID lock
// acquisition order domain.ReceivePurchaseOrder documents as load-bearing
// for avoiding an AB-BA deadlock between two concurrent receipts sharing a
// subset of stock items) is deliberate, not incidental.
const selectPurchaseOrderItemsSQL = `
	SELECT stock_item_id, (quantity * 1000)::bigint, (cost_per_unit * 100)::bigint
	FROM purchase_order_items
	WHERE purchase_order_id = $1
	ORDER BY stock_item_id ASC
`

// purchaseOrderRow is satisfied by pgx.Row, the shape both FindByID's and
// FindByIDForUpdate's single-row queries return.
type purchaseOrderRow interface {
	Scan(dest ...any) error
}

// scanPurchaseOrderRow scans a single purchase_orders row (in the exact
// column order selectPurchaseOrderColumns selects), loads its items via a
// second query against the SAME scope (and therefore the same Postgres
// transaction -- see FindByIDForUpdate's own doc comment for what "na
// mesma ida" means here), and rehydrates the whole thing into a
// *domain.PurchaseOrder via domain.NewPurchaseOrder plus the fields that
// constructor cannot know about (Status, Version, CreatedAt, ReceivedAt) --
// see hydrate.go's doc comment for why that requires setUnexportedField.
func (r *PurchaseOrderRepository) scanPurchaseOrderRow(ctx context.Context, row purchaseOrderRow, tenantID tenant.TenantID) (*domain.PurchaseOrder, error) {
	var (
		idStr         string
		supplierIDStr string
		statusStr     string
		version       int64
		createdAt     time.Time
		receivedAt    *time.Time
	)

	if err := row.Scan(&idStr, &supplierIDStr, &statusStr, &version, &createdAt, &receivedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: parsing purchase order id: %w", err)
	}
	supplierID, err := domain.ParseID(supplierIDStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: parsing purchase order supplier id: %w", err)
	}
	status, err := domain.ParsePurchaseOrderStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: parsing purchase order status: %w", err)
	}

	items, err := r.loadPurchaseOrderItems(ctx, idStr)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewPurchaseOrder(id, tenantID, supplierID, items)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: rehydrating purchase order: %w", err)
	}

	setUnexportedField(&order, "status", status)
	setUnexportedField(&order, "version", version)
	setUnexportedField(&order, "createdAt", createdAt.UTC())
	if receivedAt != nil {
		utc := receivedAt.UTC()
		setUnexportedField(&order, "receivedAt", &utc)
	} else {
		setUnexportedField(&order, "receivedAt", (*time.Time)(nil))
	}

	return &order, nil
}

// loadPurchaseOrderItems loads every purchase_order_items row for
// purchaseOrderID, rehydrating each into a domain.PurchaseOrderItem.
// PurchaseOrderItem is a plain value object (see domain/purchase_order.go)
// -- every one of its fields is a direct constructor argument, with
// nothing auto-computed the way StockItem.Version/UpdatedAt or
// PurchaseOrder.Status/CreatedAt are -- so, unlike scanPurchaseOrderRow
// above, this needs no setUnexportedField call at all.
func (r *PurchaseOrderRepository) loadPurchaseOrderItems(ctx context.Context, purchaseOrderIDStr string) ([]domain.PurchaseOrderItem, error) {
	rows, err := r.scope.Query(ctx, selectPurchaseOrderItemsSQL, purchaseOrderIDStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: querying purchase order items: %w", err)
	}
	defer rows.Close()

	var items []domain.PurchaseOrderItem
	for rows.Next() {
		var (
			stockItemIDStr string
			quantityRaw    int64
			costRaw        int64
		)
		if err := rows.Scan(&stockItemIDStr, &quantityRaw, &costRaw); err != nil {
			return nil, fmt.Errorf("operations/postgres: scanning purchase order item row: %w", err)
		}

		stockItemID, err := domain.ParseID(stockItemIDStr)
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: parsing purchase order item stock item id: %w", err)
		}
		quantity, err := domain.NewQuantity(quantityRaw)
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: building purchase order item quantity: %w", err)
		}
		cost, err := domain.NewMoney(costRaw)
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: building purchase order item cost per unit: %w", err)
		}

		item, err := domain.NewPurchaseOrderItem(stockItemID, quantity, cost)
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: rehydrating purchase order item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("operations/postgres: iterating purchase order item rows: %w", err)
	}

	return items, nil
}

// FindByID returns the PurchaseOrder identified by id without taking a row
// lock -- the read-only path, used by plain getters (see
// application.GetPurchaseOrder).
func (r *PurchaseOrderRepository) FindByID(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectPurchaseOrderColumns+" FROM purchase_orders WHERE id = $1", id.String())

	order, err := r.scanPurchaseOrderRow(ctx, row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, purchaseOrderNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: finding purchase order by id: %w", err)
	}
	return order, nil
}

// FindByIDForUpdate returns the PurchaseOrder identified by id, taking a
// row lock ("SELECT ... FOR UPDATE") so that two concurrent receipts of
// the same order serialize instead of racing -- see
// domain.PurchaseOrderRepository's doc comment.
//
// "na mesma ida" (T5.4's requirement that this method load the order's
// items in the same round trip): FindByIDForUpdate performs both the
// locking SELECT on purchase_orders and the SELECT on purchase_order_items
// within this single method call, against the very same db.Scope -- i.e.
// the very same, already-open Postgres transaction -- so a caller never
// needs (and has no way) to make a second call to fetch an order's items
// separately. It is not a single literal SQL statement (a JOIN would need
// "FOR UPDATE OF purchase_orders" to avoid also locking every matching
// purchase_order_items row, which is unnecessary: nothing ever writes to
// purchase_order_items after a purchase order's Insert -- see
// 0004_operations.sql's own comment on that table), but it is exactly one
// FindByIDForUpdate call, one transaction, no interleaved round trip a
// caller could observe or skip.
func (r *PurchaseOrderRepository) FindByIDForUpdate(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectPurchaseOrderColumns+" FROM purchase_orders WHERE id = $1 FOR UPDATE", id.String())

	order, err := r.scanPurchaseOrderRow(ctx, row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, purchaseOrderNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: finding purchase order by id for update: %w", err)
	}
	return order, nil
}

// Insert persists a brand-new PurchaseOrder, along with every one of its
// line items.
func (r *PurchaseOrderRepository) Insert(ctx context.Context, order *domain.PurchaseOrder) error {
	var receivedAtArg any
	if ra := order.ReceivedAt(); ra != nil {
		receivedAtArg = *ra
	}

	if _, err := r.scope.Exec(ctx, `
		INSERT INTO purchase_orders (id, tenant_id, supplier_id, status, version, created_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		order.ID().String(), r.scope.TenantID().String(), order.SupplierID().String(),
		string(order.Status()), order.Version(), order.CreatedAt(), receivedAtArg,
	); err != nil {
		return fmt.Errorf("operations/postgres: inserting purchase order: %w", err)
	}

	for _, item := range order.Items() {
		if _, err := r.scope.Exec(ctx, `
			INSERT INTO purchase_order_items (tenant_id, purchase_order_id, stock_item_id, quantity, cost_per_unit)
			VALUES ($1, $2, $3, $4::numeric / 1000, $5::numeric / 100)
		`,
			r.scope.TenantID().String(), order.ID().String(), item.StockItemID().String(),
			item.Quantity().Thousandths(), item.CostPerUnit().Cents(),
		); err != nil {
			return fmt.Errorf("operations/postgres: inserting purchase order item: %w", err)
		}
	}

	return nil
}

// UpdateWithVersion persists order's current status/received_at,
// enforcing optimistic concurrency against expectedVersion. The extra
// "AND status = 'pendente'" predicate is redundant with
// PurchaseOrder.MarkAsReceived's own in-memory guard (an already-received
// or cancelled order never reaches this call in the first place -- see
// domain.ReceivePurchaseOrder), but is deliberate defense in depth at the
// database layer: even a hypothetical caller bug that skipped the domain
// guard could never flip an already-received/cancelled order back to
// "recebido" through this statement.
//
// Line items are never re-written here: PurchaseOrder.Items() never
// changes once an order is created (see loadPurchaseOrderItems' own doc
// comment), so there is nothing for UpdateWithVersion to persist for
// purchase_order_items.
func (r *PurchaseOrderRepository) UpdateWithVersion(ctx context.Context, order *domain.PurchaseOrder, expectedVersion int64) error {
	var receivedAtArg any
	if ra := order.ReceivedAt(); ra != nil {
		receivedAtArg = *ra
	}

	tag, err := r.scope.Exec(ctx, `
		UPDATE purchase_orders
		SET status = $1, received_at = $2, version = version + 1
		WHERE id = $3 AND version = $4 AND status = 'pendente'
	`, string(order.Status()), receivedAtArg, order.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("operations/postgres: updating purchase order: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return purchaseOrderConflictErr()
	}
	return nil
}
