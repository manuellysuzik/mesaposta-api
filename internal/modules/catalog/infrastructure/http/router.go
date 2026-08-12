package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// Mount registers every catalog module route under r. The router is
// expected to already have auth.Authenticate middleware applied (see
// internal/modules/README.md, "What a module reuses from
// internal/platform/**") -- Mount itself does not apply any auth
// middleware, mirroring
// internal/modules/operations/infrastructure/http's and
// internal/modules/crm/infrastructure/http's own Mount. Wiring Mount into
// the real composition root (internal/platform/app.NewApp) is explicitly
// out of scope for this module and is done centrally later.
func Mount(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", listActiveProductsHandler(uow))
		r.Get("/{id}", getProductHandler(uow))
		r.Post("/", createProductHandler(uow))
		r.Post("/{id}/discount", applyDiscountHandler(uow))
	})
	r.Route("/recipes", func(r chi.Router) {
		r.Get("/{id}", getRecipeHandler(uow))
		r.Post("/", createRecipeHandler(uow))
		r.Post("/{id}/promote", promoteRecipeHandler(uow))
	})
}
