package pricing

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// MenuReader is the contracts.MenuReader implementation backed by a
// catalog domain.UnitOfWork -- the same unit of work catalog's own
// application layer uses, so listing the active menu goes through the
// exact same tenant-scoped, row-level-security-enforcing path every other
// catalog read does. See ProductPriceReader's own doc comment (pricing.go)
// for the identical rationale, one type over.
type MenuReader struct {
	uow domain.UnitOfWork
}

// compile-time check: *MenuReader must satisfy contracts.MenuReader.
var _ contracts.MenuReader = (*MenuReader)(nil)

// NewMenuReader builds a MenuReader backed by uow.
func NewMenuReader(uow domain.UnitOfWork) *MenuReader {
	return &MenuReader{uow: uow}
}

// ListActiveMenu implements contracts.MenuReader, delegating to
// application.ListActiveProducts (the same use case catalog's own
// GET /v1/products uses) and mapping the result onto the contract's
// primitive-only MenuItem, using CurrentPrice (base price with any
// discount already applied) -- exactly the price a diner should actually
// be charged, mirroring GetProductPrice's identical choice.
func (r *MenuReader) ListActiveMenu(ctx context.Context) ([]contracts.MenuItem, error) {
	products, err := application.ListActiveProducts(ctx, r.uow)
	if err != nil {
		return nil, err
	}

	items := make([]contracts.MenuItem, len(products))
	for i, product := range products {
		items[i] = contracts.MenuItem{
			ProductID:   product.ID().String(),
			Name:        product.Name(),
			Description: product.Description(),
			PriceCents:  product.CurrentPrice().Cents(),
		}
	}
	return items, nil
}
