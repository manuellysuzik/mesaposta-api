package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// RecipeIngredient is a single line describing how much of an ingredient a
// Recipe requires. It is a value object with no identity of its own --
// it is only ever meaningful as part of the Recipe that contains it. Like
// ProductIngredient, IngredientID is an opaque reference this package
// never validates against another module's data (see doc.go).
type RecipeIngredient struct {
	ingredientID ID
	quantity     Quantity
}

// NewRecipeIngredient builds a RecipeIngredient, validating that
// ingredientID is not the zero ID and quantity is non-zero.
func NewRecipeIngredient(ingredientID ID, quantity Quantity) (RecipeIngredient, error) {
	if ingredientID.IsZero() {
		return RecipeIngredient{}, errs.New(errs.Invalid, "invalid_recipe_ingredient_id",
			"recipe ingredient id must not be zero", nil)
	}
	if quantity.Thousandths() == 0 {
		return RecipeIngredient{}, errs.New(errs.Invalid, "invalid_recipe_ingredient_quantity",
			"recipe ingredient quantity must not be zero", nil)
	}
	return RecipeIngredient{ingredientID: ingredientID, quantity: quantity}, nil
}

// IngredientID returns the ID of the ingredient this line refers to.
func (i RecipeIngredient) IngredientID() ID { return i.ingredientID }

// Quantity returns the (always non-zero) quantity required.
func (i RecipeIngredient) Quantity() Quantity { return i.quantity }

// Recipe is a preparation template in a tenant's catalog: a named
// composition of ingredients that can later be promoted into a sellable
// Product (see PromoteToProduct below). It is an aggregate root guarded by
// an optimistic-concurrency Version; this phase exposes no mutation beyond
// construction, so Version never advances past 1 -- see
// internal/modules/README.md's YAGNI guidance and this module's own scope
// notes (BACKLOG.md) for why a Recipe-editing use case is deliberately not
// built yet.
type Recipe struct {
	id          ID
	tenantID    tenant.TenantID
	name        string
	description string
	ingredients []RecipeIngredient
	version     int64
	createdAt   time.Time
	updatedAt   time.Time
}

// initialRecipeVersion is the Version a freshly constructed Recipe starts
// at.
const initialRecipeVersion = 1

// NewRecipe builds a Recipe, validating that name is non-empty and that
// ingredients contains no two lines referencing the same ingredient.
//
// Unlike NewPurchaseOrder (which requires at least one item) or
// NewProduct's own duplicate-ingredient check, NewRecipe deliberately
// allows an EMPTY ingredients list: a Recipe may exist as a draft, still
// being composed, before it is ready to be sold. The "must have at least
// one ingredient" rule is instead enforced by PromoteToProduct below, at
// the point a Recipe actually becomes a sellable Product -- see that
// method's doc comment for the full rationale.
//
// ingredients is defensively copied so that later mutation of the
// caller's slice cannot reach back into the constructed Recipe.
func NewRecipe(
	id ID,
	tenantID tenant.TenantID,
	name string,
	description string,
	ingredients []RecipeIngredient,
) (Recipe, error) {
	if name == "" {
		return Recipe{}, errs.New(errs.Invalid, "invalid_recipe_name",
			"recipe name must not be empty", nil)
	}
	if err := ensureNoDuplicateRecipeIngredients(ingredients); err != nil {
		return Recipe{}, err
	}

	ingredientsCopy := make([]RecipeIngredient, len(ingredients))
	copy(ingredientsCopy, ingredients)

	now := time.Now().UTC()
	return Recipe{
		id:          id,
		tenantID:    tenantID,
		name:        name,
		description: description,
		ingredients: ingredientsCopy,
		version:     initialRecipeVersion,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// ensureNoDuplicateRecipeIngredients returns an *errs.Error of Kind
// errs.Invalid if ingredients contains two lines referencing the same
// ingredient ID.
func ensureNoDuplicateRecipeIngredients(ingredients []RecipeIngredient) error {
	seen := make(map[ID]struct{}, len(ingredients))
	for _, ing := range ingredients {
		if _, dup := seen[ing.ingredientID]; dup {
			return errs.New(errs.Invalid, "duplicate_recipe_ingredient",
				"recipe must not repeat the same ingredient across multiple lines", nil)
		}
		seen[ing.ingredientID] = struct{}{}
	}
	return nil
}

// ID returns the recipe's identity.
func (r Recipe) ID() ID { return r.id }

// TenantID returns the tenant the recipe belongs to.
func (r Recipe) TenantID() tenant.TenantID { return r.tenantID }

// Name returns the recipe's display name.
func (r Recipe) Name() string { return r.name }

// Description returns the recipe's description.
func (r Recipe) Description() string { return r.description }

// Ingredients returns a defensive copy of the recipe's ingredient lines --
// the caller cannot mutate the aggregate's internal slice through it.
func (r Recipe) Ingredients() []RecipeIngredient {
	out := make([]RecipeIngredient, len(r.ingredients))
	copy(out, r.ingredients)
	return out
}

// Version returns the recipe's optimistic-concurrency version.
func (r Recipe) Version() int64 { return r.version }

// CreatedAt returns the timestamp the recipe was created at.
func (r Recipe) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt returns the timestamp of the recipe's last successful
// mutation (or its construction time, if never mutated).
func (r Recipe) UpdatedAt() time.Time { return r.updatedAt }

// PromoteToProduct builds a brand-new Product from r, priced at
// basePrice, mapping every RecipeIngredient into an equivalent
// ProductIngredient (same ingredient ID, same quantity). It performs zero
// I/O and does not mutate r in any way -- the same Recipe may be promoted
// more than once, e.g. into two differently priced Products, since this
// phase has no requirement (and BACKLOG.md tracks none) that a Recipe be
// "consumed" by promotion; a future phase MAY choose to add that
// constraint, but nothing here assumes it.
//
// PromoteToProduct enforces the module's core promotion invariant: a
// Recipe with zero ingredients cannot be promoted (an *errs.Error of Kind
// errs.Invalid) -- unlike NewRecipe itself, which allows an empty
// ingredients list for a still-being-composed draft (see NewRecipe's doc
// comment). Every other Product invariant (non-empty name, strictly
// positive basePrice, no duplicate ingredients) is enforced by NewProduct,
// which this method delegates to for the actual construction.
func (r Recipe) PromoteToProduct(productID ID, basePrice Money) (Product, error) {
	if len(r.ingredients) == 0 {
		return Product{}, errs.New(errs.Invalid, "recipe_has_no_ingredients",
			"recipe must have at least one ingredient to be promoted to a product", nil)
	}

	productIngredients := make([]ProductIngredient, len(r.ingredients))
	for i, ing := range r.ingredients {
		productIngredient, err := NewProductIngredient(ing.ingredientID, ing.quantity)
		if err != nil {
			return Product{}, err
		}
		productIngredients[i] = productIngredient
	}

	return NewProduct(productID, r.tenantID, r.name, r.description, basePrice, productIngredients)
}
