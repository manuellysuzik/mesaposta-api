// Package pricing provides catalog's implementation of
// contracts.ProductPriceReader, adapting the catalog module's own
// domain.UnitOfWork/application.GetProduct machinery to the
// primitives-only port other modules (e.g. ordering) depend on -- see
// internal/modules/catalog/contracts/pricing.go's own doc comment.
package pricing

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// ProductPriceReader is the contracts.ProductPriceReader implementation
// backed by a catalog domain.UnitOfWork -- the same unit of work catalog's
// own application layer uses, so a price read goes through the exact same
// tenant-scoped, row-level-security-enforcing path every other catalog
// read does (see application.GetProduct's own doc comment).
type ProductPriceReader struct {
	uow domain.UnitOfWork
}

// compile-time check: *ProductPriceReader must satisfy
// contracts.ProductPriceReader.
var _ contracts.ProductPriceReader = (*ProductPriceReader)(nil)

// NewProductPriceReader builds a ProductPriceReader backed by uow.
func NewProductPriceReader(uow domain.UnitOfWork) *ProductPriceReader {
	return &ProductPriceReader{uow: uow}
}

// GetProductPrice implements contracts.ProductPriceReader. It parses
// productID via domain.ParseID (surfacing an errs.Invalid *errs.Error
// unchanged for a malformed id), loads the Product via
// application.GetProduct (surfacing its errs.NotFound *errs.Error
// unchanged for an id that does not exist, or is not visible to the
// caller's tenant), and maps the result onto the contract's primitive-only
// ProductPrice.
func (r *ProductPriceReader) GetProductPrice(ctx context.Context, productID string) (contracts.ProductPrice, error) {
	id, err := domain.ParseID(productID)
	if err != nil {
		return contracts.ProductPrice{}, err
	}

	product, err := application.GetProduct(ctx, r.uow, id)
	if err != nil {
		return contracts.ProductPrice{}, err
	}

	return contracts.ProductPrice{
		ProductID:  product.ID().String(),
		PriceCents: product.CurrentPrice().Cents(),
		IsActive:   product.IsActive(),
	}, nil
}
