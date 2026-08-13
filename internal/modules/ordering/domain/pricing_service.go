package domain

import "github.com/manuellysuzik/mesaposta-api/internal/platform/errs"

// RequestedLine is one line of a customer's cart, before any pricing has
// been resolved: which product, and how many. QuantityCount is a raw,
// UNVALIDATED int64 (not yet a Quantity) because this is exactly the shape
// the application layer decodes off an untrusted public HTTP request body
// -- PriceOrderLines is what validates it.
type RequestedLine struct {
	ProductID     string
	QuantityCount int64
}

// PriceSnapshot is the price/availability of a single product, ALREADY
// resolved by the application layer -- via
// internal/modules/catalog/contracts.ProductPriceReader -- BEFORE
// PriceOrderLines is ever called. This type is deliberately a small,
// LOCAL type, not catalog/contracts.ProductPrice itself: even though
// architecture rule R3 would permit this package to import
// catalog/contracts directly (contracts packages are the one cross-module
// import every layer, including domain/, may use), this module's domain/
// stays fully self-contained and untyped by any OTHER module's types --
// the application layer is what translates catalog's
// contracts.ProductPrice into this local shape at the boundary. This
// mirrors the wider precedent already established across this codebase:
// every module owns its own copy of small value objects rather than
// sharing types across module boundaries (see id.go/money.go/quantity.go's
// own doc comments for the identical rationale, one layer up).
type PriceSnapshot struct {
	// PriceCents is the product's CURRENT price, in cents.
	PriceCents int64
	// Active reports whether the product is currently sellable.
	Active bool
}

// PriceOrderLines is ordering's domain pricing service: PURE, no I/O (see
// this package's own doc comment, and architecture rule R1). It turns
// lines (the customer's requested cart) into frozen, priced OrderItems,
// using prices (a snapshot the CALLER already resolved, keyed by product
// id) as the sole source of truth for what each product currently costs --
// the frozen unit price this function stamps onto each OrderItem is NEVER
// taken from lines, and is never recalculated later (see Order's own doc
// comment on why prices are frozen at creation time and stay that way for
// the order's entire lifetime).
//
// A line whose ProductID is absent from prices (the catalog has no such
// product, or the caller's upstream price lookup for it came back
// NotFound) or whose snapshot has Active == false is rejected outright,
// with an *errs.Error of Kind errs.Invalid -- from the client's
// perspective this is exactly like submitting an invalid cart, not a 404:
// the ORDER endpoint exists and works, one of its ITEMS does not.
func PriceOrderLines(lines []RequestedLine, prices map[string]PriceSnapshot) ([]OrderItem, error) {
	items := make([]OrderItem, 0, len(lines))
	for _, line := range lines {
		snapshot, ok := prices[line.ProductID]
		if !ok {
			return nil, errs.New(errs.Invalid, "order_item_product_not_available",
				"order item references a product that is not available", nil)
		}
		if !snapshot.Active {
			return nil, errs.New(errs.Invalid, "order_item_product_inactive",
				"order item references a product that is not currently active", nil)
		}

		quantity, err := NewQuantity(line.QuantityCount)
		if err != nil {
			return nil, err
		}
		unitPrice, err := NewMoney(snapshot.PriceCents)
		if err != nil {
			return nil, err
		}

		item, err := NewOrderItem(line.ProductID, quantity, unitPrice)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
