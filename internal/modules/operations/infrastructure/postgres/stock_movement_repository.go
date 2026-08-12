package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// StockMovementRepository is the Postgres-backed implementation of
// domain.StockMovementRepository, backed by the (append-only)
// stock_movements table (see
// internal/platform/db/migrations/0004_operations.sql). It exposes only
// Insert, matching domain.StockMovementRepository's own append-only shape
// -- there is no update/delete method to implement, and the underlying
// table's own GRANT (SELECT, INSERT only, no UPDATE/DELETE) makes that
// immutability true at the database layer too, not merely at this Go
// type's method set.
type StockMovementRepository struct {
	scope *db.Scope
}

// compile-time check: *StockMovementRepository must satisfy domain.StockMovementRepository.
var _ domain.StockMovementRepository = (*StockMovementRepository)(nil)

// NewStockMovementRepository builds a StockMovementRepository bound to
// scope -- see StockItemRepository's NewStockItemRepository doc comment
// for why this constructor exists alongside UnitOfWork.Do/Repositories.
// StockMovements().
func NewStockMovementRepository(scope *db.Scope) *StockMovementRepository {
	return &StockMovementRepository{scope: scope}
}

// Insert persists a brand-new StockMovement.
func (r *StockMovementRepository) Insert(ctx context.Context, movement domain.StockMovement) error {
	_, err := r.scope.Exec(ctx, `
		INSERT INTO stock_movements (id, tenant_id, stock_item_id, type, quantity, reason, created_at)
		VALUES ($1, $2, $3, $4, $5::numeric / 1000, $6, $7)
	`,
		movement.ID().String(), r.scope.TenantID().String(), movement.StockItemID().String(),
		string(movement.MovementType()), movement.Quantity().Thousandths(), movement.Reason(), movement.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("operations/postgres: inserting stock movement: %w", err)
	}
	return nil
}
