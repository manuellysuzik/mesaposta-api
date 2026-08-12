package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateStockItemInput carries everything CreateStockItem needs to build a
// brand-new StockItem.
type CreateStockItemInput struct {
	TenantID    tenant.TenantID
	Name        string
	Unit        domain.StockUnit
	Quantity    domain.Quantity
	MinQuantity domain.Quantity
	CostPerUnit domain.Money
	SupplierID  *domain.ID
}

// CreateStockItem is the application-layer use case that creates a new
// StockItem. It builds the aggregate via domain.NewStockItem -- which
// performs all field validation -- and, only if that succeeds, persists it
// via StockItems().Insert within a single UnitOfWork.Do call.
func CreateStockItem(ctx context.Context, uow domain.UnitOfWork, input CreateStockItemInput) (domain.StockItem, error) {
	item, err := domain.NewStockItem(
		domain.NewID(),
		input.TenantID,
		input.Name,
		input.Unit,
		input.Quantity,
		input.MinQuantity,
		input.CostPerUnit,
		input.SupplierID,
	)
	if err != nil {
		return domain.StockItem{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.StockItems().Insert(ctx, &item)
	})
	if err != nil {
		return domain.StockItem{}, err
	}

	return item, nil
}
