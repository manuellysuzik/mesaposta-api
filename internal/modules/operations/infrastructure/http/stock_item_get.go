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
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		item, err := application.GetStockItem(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newStockItemResponse(item), nil)
	}
}
