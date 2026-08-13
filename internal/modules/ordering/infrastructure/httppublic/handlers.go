package httppublic

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// getCardapioHandler returns a handler for
// GET /public/v1/{tenantSlug}/cardapio/mesa/{code}. tenantID is expected
// to already be attached to the request context by
// internal/platform/auth/publictenant.Middleware -- this handler never
// parses {tenantSlug} itself (architecture rule R6 reserves tenant.Parse
// for internal/platform/auth/**).
//
// THE security-critical behavior this handler is responsible for: whether
// {code} does not exist at all, or exists but its table is inactive,
// application.ResolveTableForMenu reports BOTH as the exact same
// cardapioUnavailableErr -- see that function's own doc comment -- so the
// HTTP response this handler writes for either case is byte-for-byte
// identical (see handlers_test.go's own dedicated proof of this).
func getCardapioHandler(uow domain.UnitOfWork, menuReader catalogcontracts.MenuReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		code := chi.URLParam(r, "code")

		result, err := application.ResolveTableForMenu(r.Context(), uow, tenantID, code)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		menu, err := menuReader.ListActiveMenu(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newCardapioResponse(result, menu), nil)
	}
}

// createPublicOrderHandler returns a handler for
// POST /public/v1/{tenantSlug}/pedidos. It requires a valid table session
// token (see tableSessionHeader) and NEVER accepts a price from the
// client -- every price is resolved server-side, via priceReader, inside
// application.CreateOrder.
func createPublicOrderHandler(uow domain.UnitOfWork, priceReader catalogcontracts.ProductPriceReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		sessionToken := r.Header.Get(tableSessionHeader)

		req, err := decodeJSON[createPublicOrderRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		items := make([]application.CreateOrderItemInput, len(req.Items))
		for i, item := range req.Items {
			items[i] = application.CreateOrderItemInput{ProductID: item.ProductID, QuantityCount: item.Quantity}
		}

		order, err := application.CreateOrder(r.Context(), uow, priceReader, application.CreateOrderInput{
			TenantID:      tenantID,
			SessionToken:  sessionToken,
			Items:         items,
			CustomerName:  req.Name,
			CustomerPhone: req.Phone,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, createPublicOrderResponse{
			ID:     order.ID().String(),
			Status: string(order.Status()),
		}, nil)
	}
}
