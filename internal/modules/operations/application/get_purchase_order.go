package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// GetPurchaseOrder is the application-layer use case reading a single
// PurchaseOrder by id, via PurchaseOrderRepository.FindByID (the read-only,
// non-locking counterpart to FindByIDForUpdate -- see
// domain.PurchaseOrderRepository's doc comment for why that method exists
// on the port at all).
//
// Like GetStockItem, GetPurchaseOrder still goes through UnitOfWork.Do
// rather than reaching directly for a standalone repository -- see
// GetStockItem's doc comment for why that uniformity (every access, read
// or write, through the same seam) matters for a future Postgres-backed,
// row-level-security-enforcing UnitOfWork implementation.
func GetPurchaseOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.PurchaseOrder, error) {
	var order *domain.PurchaseOrder
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.PurchaseOrders().FindByID(ctx, id)
		if err != nil {
			return err
		}
		order = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}
