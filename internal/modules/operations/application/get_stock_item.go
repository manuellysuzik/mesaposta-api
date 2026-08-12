package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// GetStockItem is the application-layer use case reading a single
// StockItem by id.
//
// It still goes through UnitOfWork.Do rather than reaching directly for a
// standalone repository -- even though this is a plain read with nothing
// to roll back. This is intentional, not an accident: in the real
// Postgres-backed UnitOfWork implementation (a later phase), Do is where
// the tenant-scoped, row-level-security-enforcing database session gets
// established (see internal/platform/db.Pool.Scope). A getter that bypassed
// Do to read some other way would bypass RLS along with it -- so every
// read, not just every write, is required to flow through the same
// UnitOfWork/Repositories seam.
func GetStockItem(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.StockItem, error) {
	var item *domain.StockItem
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.StockItems().FindByID(ctx, id)
		if err != nil {
			return err
		}
		item = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}
