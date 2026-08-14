package http

import (
	"net/http"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// getProductHandler returns a handler for GET /v1/products/{id}.
func getProductHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		product, err := application.GetProduct(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newProductResponse(product), nil)
	}
}

// listActiveProductsHandler returns a handler for GET /v1/products.
func listActiveProductsHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		products, err := application.ListActiveProducts(r.Context(), uow)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		out := make([]productResponse, len(products))
		for i, p := range products {
			out[i] = newProductResponse(p)
		}

		httpx.RespondData(w, r, http.StatusOK, out, nil)
	}
}

// createProductHandler returns a handler for POST /v1/products.
func createProductHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createProductRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		basePrice, err := domain.NewMoney(req.BasePriceCents)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		ingredients, err := decodeProductIngredients(req.Ingredients)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		product, err := application.CreateProduct(r.Context(), uow, application.CreateProductInput{
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			BasePrice:   basePrice,
			Ingredients: ingredients,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newProductResponse(&product), nil)
	}
}

// applyDiscountHandler returns a handler for
// POST /v1/products/{id}/discount.
//
// The critical response shape this handler is responsible for: when
// application.ApplyDiscount rejects a discount that would drive the
// product's final price to zero or below (Kind errs.Invalid),
// httpx.RespondError maps that to HTTP 400 -- see
// internal/platform/httpx/errormap.go's kindStatus table -- with nothing
// persisted, per domain.Product.ApplyDiscount's own doc comment.
func applyDiscountHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		req, err := decodeJSON[applyDiscountRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		discount, err := domain.NewDiscountPercent(req.DiscountPercentHundredths)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		product, err := application.ApplyDiscount(r.Context(), uow, application.ApplyDiscountInput{
			ProductID: id,
			Discount:  discount,
		}, time.Now().UTC(), auth.ActorIDFrom(r.Context()))
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newProductResponse(&product), nil)
	}
}

// decodeProductIngredients maps the request-level ingredient lines into
// domain.ProductIngredient values, surfacing the first validation failure
// (invalid ingredient id, zero quantity, ...) as-is.
func decodeProductIngredients(lines []ingredientLineRequest) ([]domain.ProductIngredient, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	out := make([]domain.ProductIngredient, len(lines))
	for i, line := range lines {
		ingredientID, err := domain.ParseID(line.IngredientID)
		if err != nil {
			return nil, err
		}
		quantity, err := domain.NewQuantity(line.QuantityThousandths)
		if err != nil {
			return nil, err
		}
		ingredient, err := domain.NewProductIngredient(ingredientID, quantity)
		if err != nil {
			return nil, err
		}
		out[i] = ingredient
	}
	return out, nil
}
