package domain

import "github.com/manuellysuzik/mesaposta-api/internal/platform/errs"

// OrderItem is a single line of an Order: how many of a given catalog
// product were ordered, and at what price PER UNIT -- FROZEN at the moment
// the line was priced (see pricing_service.go), never recalculated
// afterwards even if the referenced product's catalog price later changes.
// It is a value object with no identity of its own -- it is only ever
// meaningful as part of the Order that contains it.
//
// ProductID is a plain string, not this module's own ID type: it
// identifies a Product owned by a DIFFERENT module (catalog), and this
// module deliberately never imports catalog/domain (R3) -- see
// pricing_service.go's own doc comment for the full reasoning. It is not
// validated as a well-formed UUID here; that already happened, upstream,
// the moment the application layer resolved it to a real
// catalog/contracts.ProductPrice via ProductPriceReader.
type OrderItem struct {
	productID string
	quantity  Quantity
	unitPrice Money
}

// NewOrderItem builds an OrderItem, validating that productID is not
// empty. quantity and unitPrice are already guaranteed valid (a positive
// count, a non-negative price) by their own constructors.
func NewOrderItem(productID string, quantity Quantity, unitPrice Money) (OrderItem, error) {
	if productID == "" {
		return OrderItem{}, errs.New(errs.Invalid, "invalid_order_item_product_id",
			"order item product id must not be empty", nil)
	}
	return OrderItem{productID: productID, quantity: quantity, unitPrice: unitPrice}, nil
}

// ProductID returns the (opaque, catalog-owned) id of the product this
// line refers to.
func (i OrderItem) ProductID() string { return i.productID }

// Quantity returns the (always positive) quantity ordered.
func (i OrderItem) Quantity() Quantity { return i.quantity }

// UnitPrice returns this line's FROZEN price per unit.
func (i OrderItem) UnitPrice() Money { return i.unitPrice }

// LineTotal returns this line's total cost: UnitPrice multiplied by
// Quantity. It returns an error (propagated from Money.MulInt) only in the
// extremely unlikely event that product would overflow an int64 number of
// cents.
func (i OrderItem) LineTotal() (Money, error) {
	return i.unitPrice.MulInt(i.quantity.Count())
}
