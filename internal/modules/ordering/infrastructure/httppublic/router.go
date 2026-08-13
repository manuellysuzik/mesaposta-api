package httppublic

import (
	"github.com/go-chi/chi/v5"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// Mount registers ordering's public digital-menu routes under r. The
// router is expected to already sit behind a rate limiter and
// internal/platform/auth/publictenant.Middleware -- see this package's own
// doc comment for the exact expected order and why. Mount itself applies
// neither.
func Mount(r chi.Router, uow domain.UnitOfWork, menuReader catalogcontracts.MenuReader, priceReader catalogcontracts.ProductPriceReader) {
	r.Get("/cardapio/mesa/{code}", getCardapioHandler(uow, menuReader))
	r.Post("/pedidos", createPublicOrderHandler(uow, priceReader))
}
