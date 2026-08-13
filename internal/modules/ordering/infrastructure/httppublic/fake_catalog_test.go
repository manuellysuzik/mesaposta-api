package httppublic_test

import (
	"context"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// fakeMenuReader and fakePriceReader are hand-written, in-memory
// implementations of catalog/contracts.MenuReader/ProductPriceReader --
// this package's cross-module dependencies (see router.go's own Mount
// signature). Both are intentionally unexported and live only in this
// test package.

type fakeMenuReader struct {
	items []catalogcontracts.MenuItem
	err   error
}

var _ catalogcontracts.MenuReader = (*fakeMenuReader)(nil)

func (r *fakeMenuReader) ListActiveMenu(_ context.Context) ([]catalogcontracts.MenuItem, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.items, nil
}

type fakePriceReader struct {
	prices map[string]catalogcontracts.ProductPrice
}

var _ catalogcontracts.ProductPriceReader = (*fakePriceReader)(nil)

func newFakePriceReader() *fakePriceReader {
	return &fakePriceReader{prices: map[string]catalogcontracts.ProductPrice{}}
}

func (r *fakePriceReader) GetProductPrice(_ context.Context, productID string) (catalogcontracts.ProductPrice, error) {
	price, ok := r.prices[productID]
	if !ok {
		return catalogcontracts.ProductPrice{}, errs.New(errs.NotFound, "product_not_found", "product not found", nil)
	}
	return price, nil
}
