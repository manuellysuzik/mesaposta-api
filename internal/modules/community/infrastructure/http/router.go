package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
)

// Mount registers every community module route under r. The router is
// expected to already have auth.Authenticate middleware applied (see
// internal/modules/README.md, "What a module reuses from
// internal/platform/**") -- Mount itself does not apply any auth
// middleware, mirroring internal/modules/operations and
// internal/modules/crm's own Mount. Wiring Mount into the real composition
// root (internal/platform/app.NewApp) is explicitly out of scope for this
// module (see this module's own task notes / BACKLOG.md) and is done
// centrally later.
func Mount(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/posts", func(r chi.Router) {
		r.Post("/", createPostHandler(uow))
		r.Get("/{id}", getPostHandler(uow))
		r.Post("/{id}/comments", commentOnPostHandler(uow))
		r.Put("/{id}/curtidas/me", likePostHandler(uow))
		r.Delete("/{id}/curtidas/me", unlikePostHandler(uow))
	})
}
