// Package http implements the HTTP surface of
// internal/platform/platformadmin: the platform-admin-only tenant
// provisioning API. Wiring Mount into the real composition root
// (internal/platform/app.NewApp) mounts it at /v1/platform, behind
// platformadmin.RequireToken -- never inside the /v1 group that carries
// internal/platform/auth.Authenticate, since this API is not scoped to any
// tenant at all.
package http

import (
	"github.com/go-chi/chi/v5"
)

// Mount registers this package's routes under r. r is expected to already
// carry platformadmin.RequireToken -- this package has no opinion on how
// its caller is authenticated, mirroring how every other module's own
// Mount takes authentication/authorization as already applied by its
// caller (see internal/platform/app.NewApp's own doc comments on
// /v1/_probe and ordering's role-differentiated routes).
func Mount(r chi.Router, store tenantAdminStore) {
	r.Post("/", createTenantHandler(store))
	r.Get("/", listTenantsHandler(store))
	r.Post("/{id}/atualizar", updateTenantDetailsHandler(store))
	r.Post("/{id}/ativar", activateTenantHandler(store))
	r.Post("/{id}/suspender", suspendTenantHandler(store))
}
