package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// getStockItemHandler returns a handler for GET /v1/stock-items/{id}.
func getStockItemHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		getStockItem := application.NewGetStockItem(uow)
		item, err := getStockItem.Execute(r.Context(), tenantID, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if item == nil {
			httpx.RespondError(w, r, httpx.NotFoundError("stock_item_not_found", "stock item not found", nil))
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newStockItemResponse(item), nil)
	}
}
