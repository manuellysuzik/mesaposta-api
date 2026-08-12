package contracts

import "time"

// Event name constants for the catalog module. Every name is
// dot-namespaced as "<aggregate_type>.<event>", with exactly one "." --
// internal/platform/outbox/postgres/postgres_publisher.go's aggregateTypeOf
// derives the domain_events.aggregate_type column from the prefix up to
// the first ".", so the prefix below must always match the aggregate the
// event describes (see events_test.go for the regression test pinning
// this down).
const (
	// EventProductCreated is published whenever a new Product is added to
	// a tenant's catalog. Payload: ProductCreatedPayload.
	EventProductCreated = "product.created"

	// EventProductDiscountApplied is published whenever a discount is
	// applied to a Product. Payload: ProductDiscountAppliedPayload.
	EventProductDiscountApplied = "product.discount_applied"

	// EventRecipePromoted is published whenever a Recipe is promoted into
	// a new sellable Product. Payload: RecipePromotedPayload.
	EventRecipePromoted = "recipe.promoted"
)

// ProductCreatedPayload is the payload for EventProductCreated. BasePrice
// is expressed as a whole-currency float (e.g. 350 cents -> 3.5), matching
// the scale internal/modules/operations/contracts' own payloads already
// use for monetary amounts.
type ProductCreatedPayload struct {
	ProductID  string    `json:"product_id"`
	Name       string    `json:"name"`
	BasePrice  float64   `json:"base_price"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ProductDiscountAppliedPayload is the payload for
// EventProductDiscountApplied. DiscountPercent is a whole-percentage float
// (e.g. 1234 hundredths -> 12.34); FinalPrice is the product's resulting
// CurrentPrice, in whole-currency units.
type ProductDiscountAppliedPayload struct {
	ProductID       string    `json:"product_id"`
	DiscountPercent float64   `json:"discount_percent"`
	FinalPrice      float64   `json:"final_price"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// RecipePromotedPayload is the payload for EventRecipePromoted.
type RecipePromotedPayload struct {
	RecipeID   string    `json:"recipe_id"`
	ProductID  string    `json:"product_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
