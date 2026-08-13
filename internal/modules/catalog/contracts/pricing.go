package contracts

import "context"

// ProductPrice is the primitive-only snapshot of a catalog Product's
// current price, exposed across module boundaries so other modules (e.g.
// ordering) never need to import catalog/domain directly -- see this
// package's own doc comment (doc.go) on why contracts/ is stdlib-only.
// PriceCents is the product's CURRENT price (base price with any applied
// discount already factored in -- see catalog/domain.Product.CurrentPrice),
// since that is the price a cross-module caller actually needs to charge.
type ProductPrice struct {
	ProductID  string
	PriceCents int64
	IsActive   bool
}

// ProductPriceReader is the port a module outside catalog implements
// against to read current product prices without depending on
// catalog/domain's types. The concrete implementation lives in
// catalog/infrastructure/pricing (built when a consuming module, e.g.
// ordering, needs it).
type ProductPriceReader interface {
	// GetProductPrice returns productID's current ProductPrice snapshot,
	// or an error (Kind errs.Invalid if productID is not a well-formed id,
	// Kind errs.NotFound if no such product is visible to the caller's
	// tenant) if it cannot be resolved.
	GetProductPrice(ctx context.Context, productID string) (ProductPrice, error)
}
