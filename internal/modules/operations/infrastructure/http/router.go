package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// Mount registers all operations module routes under the given router.
// The router is expected to already have auth.Authenticate middleware applied.
func Mount(r chi.Router, uow domain.UnitOfWork) {
	// Placeholder: routes will be added by each slice (Slice 1-6).
	// This function is called from internal/platform/app/app.go NewApp.
}
