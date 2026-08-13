package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// OrderRepository is the Postgres-backed implementation of
// domain.OrderRepository, backed by the orders and order_items tables (see
// internal/platform/db/migrations/0009_ordering.sql).
type OrderRepository struct {
	scope *db.Scope
}

// compile-time check: *OrderRepository must satisfy domain.OrderRepository.
var _ domain.OrderRepository = (*OrderRepository)(nil)

// NewOrderRepository builds an OrderRepository bound to scope. Most
// callers reach an OrderRepository via UnitOfWork.Do/Repositories.
// Orders() instead.
func NewOrderRepository(scope *db.Scope) *OrderRepository {
	return &OrderRepository{scope: scope}
}

func orderNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "order_not_found", "order not found", cause)
}

func orderConflictErr() error {
	return errs.New(errs.Conflict, "order_version_conflict", "order was concurrently modified", nil)
}

const selectOrderColumns = `id, table_id, customer_name, customer_phone, status, version, created_at, updated_at`

const selectOrderItemsSQL = `
	SELECT product_id, quantity, (unit_price * 100)::bigint
	FROM order_items
	WHERE order_id = $1
	ORDER BY product_id ASC
`

type orderRow interface {
	Scan(dest ...any) error
}

func (r *OrderRepository) scanOrderRow(ctx context.Context, row orderRow, tenantID tenant.TenantID) (*domain.Order, error) {
	var (
		idStr         string
		tableIDStr    string
		customerName  *string
		customerPhone *string
		statusStr     string
		version       int64
		createdAt     time.Time
		updatedAt     time.Time
	)

	if err := row.Scan(&idStr, &tableIDStr, &customerName, &customerPhone, &statusStr, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: parsing order id: %w", err)
	}
	tableID, err := domain.ParseID(tableIDStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: parsing order table id: %w", err)
	}
	status, err := domain.ParseOrderStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: parsing order status: %w", err)
	}

	items, err := r.loadOrderItems(ctx, idStr)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewOrder(id, tenantID, tableID, customerName, customerPhone, items)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: rehydrating order: %w", err)
	}

	setUnexportedField(&order, "status", status)
	setUnexportedField(&order, "version", version)
	setUnexportedField(&order, "createdAt", createdAt.UTC())
	setUnexportedField(&order, "updatedAt", updatedAt.UTC())

	return &order, nil
}

func (r *OrderRepository) loadOrderItems(ctx context.Context, orderIDStr string) ([]domain.OrderItem, error) {
	rows, err := r.scope.Query(ctx, selectOrderItemsSQL, orderIDStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: querying order items: %w", err)
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		var (
			productID   string
			quantityRaw int64
			priceRaw    int64
		)
		if err := rows.Scan(&productID, &quantityRaw, &priceRaw); err != nil {
			return nil, fmt.Errorf("ordering/postgres: scanning order item row: %w", err)
		}

		quantity, err := domain.NewQuantity(quantityRaw)
		if err != nil {
			return nil, fmt.Errorf("ordering/postgres: building order item quantity: %w", err)
		}
		price, err := domain.NewMoney(priceRaw)
		if err != nil {
			return nil, fmt.Errorf("ordering/postgres: building order item unit price: %w", err)
		}
		item, err := domain.NewOrderItem(productID, quantity, price)
		if err != nil {
			return nil, fmt.Errorf("ordering/postgres: rehydrating order item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ordering/postgres: iterating order item rows: %w", err)
	}
	return items, nil
}

// FindByID returns the Order identified by id, or a NotFound *errs.Error
// if none exists (or none is visible to the scope's tenant, per RLS).
func (r *OrderRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Order, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectOrderColumns+" FROM orders WHERE id = $1", id.String())

	order, err := r.scanOrderRow(ctx, row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, orderNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: finding order by id: %w", err)
	}
	return order, nil
}

// FindAll returns every Order visible to the scope's tenant, ordered
// descending by created_at (most recent first).
func (r *OrderRepository) FindAll(ctx context.Context) ([]*domain.Order, error) {
	rows, err := r.scope.Query(ctx, "SELECT "+selectOrderColumns+" FROM orders ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: querying orders: %w", err)
	}
	defer rows.Close()

	var out []*domain.Order
	for rows.Next() {
		order, err := r.scanOrderRow(ctx, rows, r.scope.TenantID())
		if err != nil {
			return nil, fmt.Errorf("ordering/postgres: scanning order row: %w", err)
		}
		out = append(out, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ordering/postgres: iterating order rows: %w", err)
	}
	return out, nil
}

// Insert persists a brand-new Order, along with every one of its line
// items.
func (r *OrderRepository) Insert(ctx context.Context, order *domain.Order) error {
	if _, err := r.scope.Exec(ctx, `
		INSERT INTO orders (id, tenant_id, table_id, customer_name, customer_phone, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		order.ID().String(), r.scope.TenantID().String(), order.TableID().String(),
		order.CustomerName(), order.CustomerPhone(), string(order.Status()),
		order.Version(), order.CreatedAt(), order.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("ordering/postgres: inserting order: %w", err)
	}

	for _, item := range order.Items() {
		if _, err := r.scope.Exec(ctx, `
			INSERT INTO order_items (tenant_id, order_id, product_id, quantity, unit_price)
			VALUES ($1, $2, $3, $4, $5::numeric / 100)
		`,
			r.scope.TenantID().String(), order.ID().String(), item.ProductID(),
			item.Quantity().Count(), item.UnitPrice().Cents(),
		); err != nil {
			return fmt.Errorf("ordering/postgres: inserting order item: %w", err)
		}
	}

	return nil
}

// UpdateWithVersion persists order's current status/updated_at, enforcing
// optimistic concurrency against expectedVersion. Line items are never
// re-written here: Order.Items() never changes once an order is created
// (see loadOrderItems' own doc comment context in
// 0009_ordering.sql), so there is nothing else for this statement to
// persist.
func (r *OrderRepository) UpdateWithVersion(ctx context.Context, order *domain.Order, expectedVersion int64) error {
	tag, err := r.scope.Exec(ctx, `
		UPDATE orders
		SET status = $1, version = version + 1, updated_at = $2
		WHERE id = $3 AND version = $4
	`, string(order.Status()), order.UpdatedAt(), order.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("ordering/postgres: updating order: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return orderConflictErr()
	}
	return nil
}
