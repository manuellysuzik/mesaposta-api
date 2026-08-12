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

// StockItemRepository is the Postgres-backed implementation of
// domain.StockItemRepository, backed by the stock_items table (see
// internal/platform/db/migrations/0004_operations.sql). Every method runs
// against an already-open db.Scope -- there is deliberately no
// "tenant_id = $n" predicate in any SELECT/UPDATE below, since Postgres
// row-level security (the tenant_isolation policy on stock_items) already
// restricts every statement to the scope's own tenant; adding a redundant
// application-level predicate would only invite the two to silently drift
// apart over time. Every rehydrated StockItem's TenantID is set from
// scope.TenantID() -- never re-parsed from the row's own tenant_id column
// (which architecture rule R6 would forbid this package from doing via
// tenant.Parse anyway) -- since RLS already guarantees that column can only
// ever equal the scope's tenant.
type StockItemRepository struct {
	scope *db.Scope
}

// compile-time check: *StockItemRepository must satisfy domain.StockItemRepository.
var _ domain.StockItemRepository = (*StockItemRepository)(nil)

// NewStockItemRepository builds a StockItemRepository bound to scope. Most
// callers reach a StockItemRepository via UnitOfWork.Do/Repositories.
// StockItems() instead; this constructor exists so this package's own
// integration test suite (and any other caller that already holds a
// *db.Scope) can exercise it directly, in isolation from the rest of
// Repositories.
func NewStockItemRepository(scope *db.Scope) *StockItemRepository {
	return &StockItemRepository{scope: scope}
}

// stockItemNotFoundErr builds the *errs.Error a missing-row lookup returns,
// mirroring internal/platform/auth/refresh.PostgresStore's notFoundErr
// pattern.
func stockItemNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "stock_item_not_found", "stock item not found", cause)
}

const selectStockItemColumns = `
	id, name, unit, (quantity * 1000)::bigint, (min_quantity * 1000)::bigint,
	(cost_per_unit * 100)::bigint, supplier_id, version, updated_at
`

// stockItemRow is satisfied by both pgx.Row (QueryRow) and pgx.Rows
// (Query.Next), factored out so scanStockItemRow can serve both FindByID/
// FindByIDForUpdate and FindManyByIDs.
type stockItemRow interface {
	Scan(dest ...any) error
}

// scanStockItemRow scans a single stock_items row (in the exact column
// order selectStockItemColumns selects) and rehydrates it into a
// *domain.StockItem via domain.NewStockItem plus the two fields that
// constructor cannot know about (Version, UpdatedAt) -- see hydrate.go's
// doc comment for why that requires setUnexportedField instead of a public
// domain constructor.
func scanStockItemRow(row stockItemRow, tenantID tenant.TenantID) (*domain.StockItem, error) {
	var (
		idStr       string
		name        string
		unitStr     string
		quantityRaw int64
		minQtyRaw   int64
		costRaw     int64
		supplierStr *string
		version     int64
		updatedAt   time.Time
	)

	if err := row.Scan(&idStr, &name, &unitStr, &quantityRaw, &minQtyRaw, &costRaw, &supplierStr, &version, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: parsing stock item id: %w", err)
	}
	unit, err := domain.ParseStockUnit(unitStr)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: parsing stock item unit: %w", err)
	}
	quantity, err := domain.NewQuantity(quantityRaw)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: building stock item quantity: %w", err)
	}
	minQuantity, err := domain.NewQuantity(minQtyRaw)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: building stock item min quantity: %w", err)
	}
	cost, err := domain.NewMoney(costRaw)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: building stock item cost per unit: %w", err)
	}
	var supplierID *domain.ID
	if supplierStr != nil {
		parsed, err := domain.ParseID(*supplierStr)
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: parsing stock item supplier id: %w", err)
		}
		supplierID = &parsed
	}

	item, err := domain.NewStockItem(id, tenantID, name, unit, quantity, minQuantity, cost, supplierID)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: rehydrating stock item: %w", err)
	}

	setUnexportedField(&item, "version", version)
	setUnexportedField(&item, "updatedAt", updatedAt.UTC())

	return &item, nil
}

// FindByID returns the StockItem identified by id, or a NotFound
// *errs.Error if none exists (or none is visible to the scope's tenant,
// per RLS).
func (r *StockItemRepository) FindByID(ctx context.Context, id domain.ID) (*domain.StockItem, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectStockItemColumns+" FROM stock_items WHERE id = $1", id.String())

	item, err := scanStockItemRow(row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, stockItemNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: finding stock item by id: %w", err)
	}
	return item, nil
}

// FindManyByIDs returns every StockItem identified by ids, keyed by ID.
// Rows are read back ordered ascending by id -- see
// purchase_order_repository.go's doc comment on FindByIDForUpdate for why
// that ordering matters for lock-ordering deadlock avoidance across the
// whole receive-purchase-order flow (domain.ReceivePurchaseOrder already
// guarantees the StockItems it hands back to be mutated follow this same
// ascending order; sorting the read here keeps every step of the flow
// consistent, even though the map this method returns is itself unordered).
func (r *StockItemRepository) FindManyByIDs(ctx context.Context, ids []domain.ID) (map[domain.ID]*domain.StockItem, error) {
	out := make(map[domain.ID]*domain.StockItem, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = id.String()
	}

	rows, err := r.scope.Query(ctx,
		"SELECT "+selectStockItemColumns+" FROM stock_items WHERE id = ANY($1) ORDER BY id ASC", idStrs)
	if err != nil {
		return nil, fmt.Errorf("operations/postgres: querying stock items by ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := scanStockItemRow(rows, r.scope.TenantID())
		if err != nil {
			return nil, fmt.Errorf("operations/postgres: scanning stock item row: %w", err)
		}
		out[item.ID()] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("operations/postgres: iterating stock item rows: %w", err)
	}

	return out, nil
}

// Insert persists a brand-new StockItem.
func (r *StockItemRepository) Insert(ctx context.Context, item *domain.StockItem) error {
	var supplierIDArg any
	if id := item.SupplierID(); id != nil {
		supplierIDArg = id.String()
	}

	_, err := r.scope.Exec(ctx, `
		INSERT INTO stock_items (id, tenant_id, name, unit, quantity, min_quantity, cost_per_unit, supplier_id, version, updated_at)
		VALUES ($1, $2, $3, $4, $5::numeric / 1000, $6::numeric / 1000, $7::numeric / 100, $8, $9, $10)
	`,
		item.ID().String(), r.scope.TenantID().String(), item.Name(), string(item.Unit()),
		item.Quantity().Thousandths(), item.MinQuantity().Thousandths(), item.CostPerUnit().Cents(),
		supplierIDArg, item.Version(), item.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("operations/postgres: inserting stock item: %w", err)
	}
	return nil
}

// stockItemConflictErr builds the *errs.Error UpdateWithVersion returns
// when its UPDATE affects zero rows -- see domain.StockItemRepository's
// doc comment for the full optimistic-concurrency contract this
// implements.
func stockItemConflictErr() error {
	return errs.New(errs.Conflict, "stock_item_version_conflict",
		"stock item was concurrently modified", nil)
}

// UpdateWithVersion persists item's current quantity, enforcing optimistic
// concurrency against expectedVersion: "WHERE id = $1 AND version = $2".
// Only quantity, version and updated_at are written -- name, unit,
// min_quantity, cost_per_unit and supplier_id never change through
// StockItem.Receive/Withdraw (the only mutations that ever call
// UpdateWithVersion), so there is nothing else for this statement to
// persist.
func (r *StockItemRepository) UpdateWithVersion(ctx context.Context, item *domain.StockItem, expectedVersion int64) error {
	tag, err := r.scope.Exec(ctx, `
		UPDATE stock_items
		SET quantity = $1::numeric / 1000, version = version + 1, updated_at = $2
		WHERE id = $3 AND version = $4
	`, item.Quantity().Thousandths(), item.UpdatedAt(), item.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("operations/postgres: updating stock item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return stockItemConflictErr()
	}
	return nil
}
