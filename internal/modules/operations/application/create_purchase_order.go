package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreatePurchaseOrderInput carries everything CreatePurchaseOrder needs to
// build a brand-new PurchaseOrder.
type CreatePurchaseOrderInput struct {
	TenantID   tenant.TenantID
	SupplierID domain.ID
	Items      []domain.PurchaseOrderItem
}

// CreatePurchaseOrder is the application-layer use case that creates a new
// PurchaseOrder. It builds the aggregate via domain.NewPurchaseOrder --
// which validates that Items is non-empty and free of duplicate stock
// items -- and, only if that succeeds, persists it via
// PurchaseOrders().Insert within a single UnitOfWork.Do call.
func CreatePurchaseOrder(ctx context.Context, uow domain.UnitOfWork, input CreatePurchaseOrderInput) (domain.PurchaseOrder, error) {
	order, err := domain.NewPurchaseOrder(domain.NewID(), input.TenantID, input.SupplierID, input.Items)
	if err != nil {
		return domain.PurchaseOrder{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.PurchaseOrders().Insert(ctx, &order)
	})
	if err != nil {
		return domain.PurchaseOrder{}, err
	}

	return order, nil
}
