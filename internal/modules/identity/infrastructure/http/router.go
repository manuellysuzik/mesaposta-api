package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
)

// Mount registers the identity module's public (unauthenticated) routes
// under r. Unlike every other module's Mount (catalog, crm, operations,
// community, ordering), r here is expected to NOT have auth.Authenticate
// applied -- a login attempt has no token yet, so requiring one would make
// login impossible. Wiring Mount into the real composition root
// (internal/platform/app.NewApp) mounts it at /v1/auth, outside the /v1
// group that does carry auth.Authenticate.
func Mount(r chi.Router, lookup application.UserCredentialsLookup, issuer *token.Issuer) {
	r.Post("/login", loginHandler(lookup, issuer))
}
