package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// Mount registers all operations module routes under the given router.
// The router is expected to already have auth.Authenticate middleware applied.
func Mount(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/stock-items", func(r chi.Router) {
		r.Get("/{id}", getStockItemHandler(uow))
		r.Post("/", createStockItemHandler(uow))
		r.Post("/{id}/movements", registerStockMovementHandler(uow))
	})
	r.Route("/purchase-orders", func(r chi.Router) {
		r.Get("/{id}", getPurchaseOrderHandler(uow))
		r.Post("/", createPurchaseOrderHandler(uow))
		r.Post("/{id}/receive", receivePurchaseOrderHandler(uow))
	})
}
