package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
)

// Mount registers every crm module route under r. The router is expected
// to already have auth.Authenticate middleware applied (see
// internal/modules/README.md, "What a module reuses from
// internal/platform/**") -- Mount itself does not apply any auth
// middleware, mirroring internal/modules/operations/infrastructure/http's
// own Mount. Wiring Mount into the real composition root
// (internal/platform/app.NewApp) is explicitly out of scope for this
// module (see this module's own task notes / BACKLOG.md) and is done
// centrally later.
func Mount(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/customers", func(r chi.Router) {
		r.Get("/{id}", getCustomerHandler(uow))
		r.Post("/", createCustomerHandler(uow))
		r.Post("/{id}/loyalty/credit", creditLoyaltyPointsHandler(uow))
		r.Post("/{id}/loyalty/redeem", redeemLoyaltyPointsHandler(uow))
	})
}
