package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// ProductIngredient is a single line describing how much of an ingredient
// a Product consumes (e.g. per unit sold). It is a value object with no
// identity of its own -- it is only ever meaningful as part of the Product
// that contains it. IngredientID is an opaque reference (today, in
// practice, a StockItem ID owned by internal/modules/operations); this
// package never validates it against operations' own data -- see doc.go
// and 0005_catalog.sql for why that cross-module check is deliberately out
// of scope for this phase.
type ProductIngredient struct {
	ingredientID ID
	quantity     Quantity
}

// NewProductIngredient builds a ProductIngredient, validating that
// ingredientID is not the zero ID and quantity is non-zero (an ingredient
// line consuming zero of something makes no sense).
func NewProductIngredient(ingredientID ID, quantity Quantity) (ProductIngredient, error) {
	if ingredientID.IsZero() {
		return ProductIngredient{}, errs.New(errs.Invalid, "invalid_product_ingredient_id",
			"product ingredient id must not be zero", nil)
	}
	if quantity.Thousandths() == 0 {
		return ProductIngredient{}, errs.New(errs.Invalid, "invalid_product_ingredient_quantity",
			"product ingredient quantity must not be zero", nil)
	}
	return ProductIngredient{ingredientID: ingredientID, quantity: quantity}, nil
}

// IngredientID returns the ID of the ingredient this line refers to.
func (i ProductIngredient) IngredientID() ID { return i.ingredientID }

// Quantity returns the (always non-zero) quantity consumed.
func (i ProductIngredient) Quantity() Quantity { return i.quantity }

// Product is a sellable item in a tenant's catalog: its name, its base
// price, an optional applied discount, and the ingredients it consumes
// (empty for a product with no tracked composition, e.g. a resold,
// unprepared item). It is an aggregate root guarded by an
// optimistic-concurrency Version, incremented on every successful
// mutation (ApplyDiscount).
type Product struct {
	id          ID
	tenantID    tenant.TenantID
	name        string
	description string
	basePrice   Money
	discount    *DiscountPercent
	ingredients []ProductIngredient
	isActive    bool
	version     int64
	createdAt   time.Time
	updatedAt   time.Time
}

// initialProductVersion is the Version a freshly constructed Product
// starts at, before any mutation.
const initialProductVersion = 1

// NewProduct builds a Product, validating that name is non-empty, that
// basePrice is STRICTLY positive (unlike Money's own constructor, which
// merely forbids negative amounts -- a product priced at exactly zero is
// not a meaningful catalog entry), and that ingredients contains no two
// lines referencing the same ingredient. The new product always starts out
// active (IsActive true), with no discount applied, at Version 1.
//
// ingredients is defensively copied so that later mutation of the
// caller's slice cannot reach back into the constructed Product.
func NewProduct(
	id ID,
	tenantID tenant.TenantID,
	name string,
	description string,
	basePrice Money,
	ingredients []ProductIngredient,
) (Product, error) {
	if name == "" {
		return Product{}, errs.New(errs.Invalid, "invalid_product_name",
			"product name must not be empty", nil)
	}
	if basePrice.Cents() <= 0 {
		return Product{}, errs.New(errs.Invalid, "invalid_product_price",
			"product price must be greater than zero", nil)
	}
	if err := ensureNoDuplicateIngredients(ingredients); err != nil {
		return Product{}, err
	}

	ingredientsCopy := make([]ProductIngredient, len(ingredients))
	copy(ingredientsCopy, ingredients)

	now := time.Now().UTC()
	return Product{
		id:          id,
		tenantID:    tenantID,
		name:        name,
		description: description,
		basePrice:   basePrice,
		ingredients: ingredientsCopy,
		isActive:    true,
		version:     initialProductVersion,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// ensureNoDuplicateIngredients returns an *errs.Error of Kind errs.Invalid
// if ingredients contains two lines referencing the same ingredient ID.
func ensureNoDuplicateIngredients(ingredients []ProductIngredient) error {
	seen := make(map[ID]struct{}, len(ingredients))
	for _, ing := range ingredients {
		if _, dup := seen[ing.ingredientID]; dup {
			return errs.New(errs.Invalid, "duplicate_product_ingredient",
				"product must not repeat the same ingredient across multiple lines", nil)
		}
		seen[ing.ingredientID] = struct{}{}
	}
	return nil
}

// ID returns the product's identity.
func (p Product) ID() ID { return p.id }

// TenantID returns the tenant the product belongs to.
func (p Product) TenantID() tenant.TenantID { return p.tenantID }

// Name returns the product's display name.
func (p Product) Name() string { return p.name }

// Description returns the product's description.
func (p Product) Description() string { return p.description }

// BasePrice returns the product's price before any discount is applied.
func (p Product) BasePrice() Money { return p.basePrice }

// Discount returns the currently applied discount, or nil if none has
// been applied.
func (p Product) Discount() *DiscountPercent { return p.discount }

// CurrentPrice returns the product's effective, sellable price: BasePrice
// with Discount applied, if any.
func (p Product) CurrentPrice() Money {
	if p.discount == nil {
		return p.basePrice
	}
	return p.basePrice.ApplyDiscount(*p.discount)
}

// Ingredients returns a defensive copy of the product's ingredient lines --
// the caller cannot mutate the aggregate's internal slice through it.
func (p Product) Ingredients() []ProductIngredient {
	out := make([]ProductIngredient, len(p.ingredients))
	copy(out, p.ingredients)
	return out
}

// IsActive returns whether the product is currently active (visible to
// ListActiveProducts).
func (p Product) IsActive() bool { return p.isActive }

// Version returns the product's optimistic-concurrency version, starting
// at 1 and incrementing by exactly 1 on every successful mutation.
func (p Product) Version() int64 { return p.version }

// CreatedAt returns the timestamp the product was created at.
func (p Product) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns the timestamp of the product's last successful
// mutation (or its construction time, if never mutated).
func (p Product) UpdatedAt() time.Time { return p.updatedAt }

// ApplyDiscount replaces the product's currently applied discount (if any)
// with discount, enforcing the invariant that the resulting CurrentPrice
// stays strictly positive: a discount that would drive the final price to
// zero or below is refused with an *errs.Error of Kind errs.Invalid, and
// the aggregate is left completely untouched (discount, version and
// updatedAt all unchanged).
func (p *Product) ApplyDiscount(discount DiscountPercent) error {
	finalPrice := p.basePrice.ApplyDiscount(discount)
	if finalPrice.Cents() <= 0 {
		return errs.New(errs.Invalid, "discount_results_in_non_positive_price",
			"applying this discount would make the product's final price zero or negative", nil)
	}

	p.discount = &discount
	p.version++
	p.updatedAt = time.Now().UTC()
	return nil
}
