package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// getRecipeHandler returns a handler for GET /v1/recipes/{id}.
func getRecipeHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		recipe, err := application.GetRecipe(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newRecipeResponse(recipe), nil)
	}
}

// createRecipeHandler returns a handler for POST /v1/recipes.
func createRecipeHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createRecipeRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		ingredients, err := decodeRecipeIngredients(req.Ingredients)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		recipe, err := application.CreateRecipe(r.Context(), uow, application.CreateRecipeInput{
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			Ingredients: ingredients,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newRecipeResponse(&recipe), nil)
	}
}

// promoteRecipeHandler returns a handler for
// POST /v1/recipes/{id}/promote.
//
// The critical response shape this handler is responsible for: when
// application.PromoteRecipeToProduct rejects a Recipe with no ingredients
// (Kind errs.Invalid), httpx.RespondError maps that to HTTP 400, with
// nothing persisted, per domain.Recipe.PromoteToProduct's own doc comment.
func promoteRecipeHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[promoteRecipeRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		basePrice, err := domain.NewMoney(req.BasePriceCents)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		product, err := application.PromoteRecipeToProduct(r.Context(), uow, application.PromoteRecipeToProductInput{
			RecipeID:  id,
			BasePrice: basePrice,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newProductResponse(&product), nil)
	}
}

// decodeRecipeIngredients maps the request-level ingredient lines into
// domain.RecipeIngredient values, surfacing the first validation failure
// as-is.
func decodeRecipeIngredients(lines []ingredientLineRequest) ([]domain.RecipeIngredient, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	out := make([]domain.RecipeIngredient, len(lines))
	for i, line := range lines {
		ingredientID, err := domain.ParseID(line.IngredientID)
		if err != nil {
			return nil, err
		}
		quantity, err := domain.NewQuantity(line.QuantityThousandths)
		if err != nil {
			return nil, err
		}
		ingredient, err := domain.NewRecipeIngredient(ingredientID, quantity)
		if err != nil {
			return nil, err
		}
		out[i] = ingredient
	}
	return out, nil
}
