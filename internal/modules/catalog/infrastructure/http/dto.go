package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// ingredientResponse is the JSON representation of a single
// ProductIngredient/RecipeIngredient line -- both value objects share the
// exact same shape (an ingredient id and a quantity), so a single response
// DTO serves both.
type ingredientResponse struct {
	IngredientID        string `json:"ingredient_id"`
	QuantityThousandths int64  `json:"quantity_thousandths"`
}

// productResponse is the JSON representation of a Product aggregate.
type productResponse struct {
	ID                        string               `json:"id"`
	TenantID                  string               `json:"tenant_id"`
	Name                      string               `json:"name"`
	Description               string               `json:"description"`
	BasePriceCents            int64                `json:"base_price_cents"`
	DiscountPercentHundredths *int64               `json:"discount_percent_hundredths"`
	CurrentPriceCents         int64                `json:"current_price_cents"`
	Ingredients               []ingredientResponse `json:"ingredients"`
	IsActive                  bool                 `json:"is_active"`
	Version                   int64                `json:"version"`
	CreatedAt                 time.Time            `json:"created_at"`
	UpdatedAt                 time.Time            `json:"updated_at"`
}

// newProductResponse maps a domain Product to its HTTP response.
func newProductResponse(p *domain.Product) productResponse {
	var discount *int64
	if d := p.Discount(); d != nil {
		h := d.Hundredths()
		discount = &h
	}

	ingredients := make([]ingredientResponse, len(p.Ingredients()))
	for i, ing := range p.Ingredients() {
		ingredients[i] = ingredientResponse{
			IngredientID:        ing.IngredientID().String(),
			QuantityThousandths: ing.Quantity().Thousandths(),
		}
	}

	return productResponse{
		ID:                        p.ID().String(),
		TenantID:                  p.TenantID().String(),
		Name:                      p.Name(),
		Description:               p.Description(),
		BasePriceCents:            p.BasePrice().Cents(),
		DiscountPercentHundredths: discount,
		CurrentPriceCents:         p.CurrentPrice().Cents(),
		Ingredients:               ingredients,
		IsActive:                  p.IsActive(),
		Version:                   p.Version(),
		CreatedAt:                 p.CreatedAt(),
		UpdatedAt:                 p.UpdatedAt(),
	}
}

// recipeResponse is the JSON representation of a Recipe aggregate.
type recipeResponse struct {
	ID          string               `json:"id"`
	TenantID    string               `json:"tenant_id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Ingredients []ingredientResponse `json:"ingredients"`
	Version     int64                `json:"version"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// newRecipeResponse maps a domain Recipe to its HTTP response.
func newRecipeResponse(r *domain.Recipe) recipeResponse {
	ingredients := make([]ingredientResponse, len(r.Ingredients()))
	for i, ing := range r.Ingredients() {
		ingredients[i] = ingredientResponse{
			IngredientID:        ing.IngredientID().String(),
			QuantityThousandths: ing.Quantity().Thousandths(),
		}
	}

	return recipeResponse{
		ID:          r.ID().String(),
		TenantID:    r.TenantID().String(),
		Name:        r.Name(),
		Description: r.Description(),
		Ingredients: ingredients,
		Version:     r.Version(),
		CreatedAt:   r.CreatedAt(),
		UpdatedAt:   r.UpdatedAt(),
	}
}

// ingredientLineRequest is the request-side shape of a single ingredient
// line, shared by createProductRequest and createRecipeRequest.
type ingredientLineRequest struct {
	IngredientID        string `json:"ingredient_id"`
	QuantityThousandths int64  `json:"quantity_thousandths"`
}

// createProductRequest is the request payload for POST /v1/products.
type createProductRequest struct {
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	BasePriceCents int64                   `json:"base_price_cents"`
	Ingredients    []ingredientLineRequest `json:"ingredients"`
}

// applyDiscountRequest is the request payload for
// POST /v1/products/{id}/discount.
type applyDiscountRequest struct {
	DiscountPercentHundredths int64 `json:"discount_percent_hundredths"`
}

// createRecipeRequest is the request payload for POST /v1/recipes.
type createRecipeRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Ingredients []ingredientLineRequest `json:"ingredients"`
}

// promoteRecipeRequest is the request payload for
// POST /v1/recipes/{id}/promote.
type promoteRecipeRequest struct {
	BasePriceCents int64 `json:"base_price_cents"`
}
