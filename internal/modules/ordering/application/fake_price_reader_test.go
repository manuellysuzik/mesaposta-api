package application_test

import (
	"context"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// fakePriceReader is a hand-written, in-memory implementation of
// catalog/contracts.ProductPriceReader -- CreateOrder's cross-module
// dependency (see create_order.go's own doc comment for why depending on
// another module's contracts/ package here is explicitly permitted by
// architecture rule R3). It is intentionally unexported and lives only in
// this test package.
type fakePriceReader struct {
	prices map[string]catalogcontracts.ProductPrice
	err    error
}

var _ catalogcontracts.ProductPriceReader = (*fakePriceReader)(nil)

func newFakePriceReader() *fakePriceReader {
	return &fakePriceReader{prices: map[string]catalogcontracts.ProductPrice{}}
}

func (r *fakePriceReader) GetProductPrice(_ context.Context, productID string) (catalogcontracts.ProductPrice, error) {
	if r.err != nil {
		return catalogcontracts.ProductPrice{}, r.err
	}
	price, ok := r.prices[productID]
	if !ok {
		return catalogcontracts.ProductPrice{}, errs.New(errs.NotFound, "product_not_found", "product not found", nil)
	}
	return price, nil
}
