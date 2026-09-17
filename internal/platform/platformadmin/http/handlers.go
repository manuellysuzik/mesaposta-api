package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// slugPattern mirrors the frontend's own slug validation
// (mesaposta-frontend's tenant-form-dialog.tsx): lowercase letters,
// digits and hyphens only. Enforced here too, not just client-side, since
// this is the actual write path -- the frontend's own check is a UX
// nicety, not a security boundary.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// newTenantID generates a fresh random UUID v4, following RFC 4122 section
// 4.4. Deliberately duplicated (not imported) from
// internal/modules/identity/domain.NewID -- see that function's own doc
// comment on why each boundary owns this small, self-contained primitive
// rather than sharing it: platformadmin has no domain layer to import from
// at all, and reaching into a business module's domain package from
// platform code would invert this codebase's intended dependency direction
// (modules depend on platform, never the other way around).
func newTenantID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("platformadmin: newTenantID: crypto/rand.Read failed: %v", err))
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	h := hex.EncodeToString(raw[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// validationErr builds the uniform *errs.Error every request-validation
// failure in this package is reported as.
func validationErr(code, message string) error {
	return errs.New(errs.Invalid, code, message, nil)
}

// validateCreateTenantRequest checks req's fields beyond what decodeJSON's
// JSON-shape validation already covers.
func validateCreateTenantRequest(req createTenantRequest) error {
	if len(req.Slug) < 2 || !slugPattern.MatchString(req.Slug) {
		return validationErr("invalid_slug", "slug must be at least 2 characters, using only lowercase letters, digits and hyphens")
	}
	if len(req.Name) < 2 {
		return validationErr("invalid_name", "name must be at least 2 characters")
	}
	if !isValidPlan(req.Plan) {
		return validationErr("invalid_plan", "plan must be one of: basic, pro, enterprise")
	}
	return nil
}

// validateUpdateTenantDetailsRequest checks req's fields beyond what
// decodeJSON's JSON-shape validation already covers.
func validateUpdateTenantDetailsRequest(req updateTenantDetailsRequest) error {
	if len(req.Name) < 2 {
		return validationErr("invalid_name", "name must be at least 2 characters")
	}
	if !isValidPlan(req.Plan) {
		return validationErr("invalid_plan", "plan must be one of: basic, pro, enterprise")
	}
	return nil
}

// tenantAdminStore is the narrow port this package's handlers depend on --
// satisfied by *internal/platform/db.Pool in production, and by a
// hand-written fake in this package's own tests, mirroring every other
// module's own fake-implements-the-port test pattern (e.g.
// internal/modules/catalog/infrastructure/http's fake_uow_test.go).
type tenantAdminStore interface {
	CreateTenant(ctx context.Context, input db.CreateTenantInput) (db.PlatformTenant, error)
	ListTenants(ctx context.Context) ([]db.PlatformTenant, error)
	UpdateTenantDetails(ctx context.Context, id string, input db.UpdateTenantDetailsInput) (db.PlatformTenant, error)
	SetTenantStatus(ctx context.Context, id, status string) (db.PlatformTenant, error)
}

// createTenantHandler returns a handler for POST /v1/platform/tenants.
func createTenantHandler(store tenantAdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := decodeJSON[createTenantRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		if err := validateCreateTenantRequest(req); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		tenant, err := store.CreateTenant(r.Context(), db.CreateTenantInput{
			ID:         newTenantID(),
			Slug:       req.Slug,
			Name:       req.Name,
			OwnerName:  req.OwnerName,
			OwnerEmail: req.OwnerEmail,
			Phone:      req.Phone,
			Plan:       req.Plan,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newTenantResponse(tenant), nil)
	}
}

// listTenantsHandler returns a handler for GET /v1/platform/tenants.
func listTenantsHandler(store tenantAdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenants, err := store.ListTenants(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		out := make([]tenantResponse, len(tenants))
		for i, t := range tenants {
			out[i] = newTenantResponse(t)
		}
		httpx.RespondData(w, r, http.StatusOK, out, nil)
	}
}

// updateTenantDetailsHandler returns a handler for POST
// /v1/platform/tenants/{id}/atualizar.
func updateTenantDetailsHandler(store tenantAdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		req, err := decodeJSON[updateTenantDetailsRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		if err := validateUpdateTenantDetailsRequest(req); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		tenant, err := store.UpdateTenantDetails(r.Context(), id, db.UpdateTenantDetailsInput{
			Name:       req.Name,
			OwnerName:  req.OwnerName,
			OwnerEmail: req.OwnerEmail,
			Phone:      req.Phone,
			Plan:       req.Plan,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newTenantResponse(tenant), nil)
	}
}

// activateTenantHandler returns a handler for POST
// /v1/platform/tenants/{id}/ativar.
func activateTenantHandler(store tenantAdminStore) http.HandlerFunc {
	return setTenantStatusHandler(store, "active")
}

// suspendTenantHandler returns a handler for POST
// /v1/platform/tenants/{id}/suspender.
func suspendTenantHandler(store tenantAdminStore) http.HandlerFunc {
	return setTenantStatusHandler(store, "suspended")
}

// setTenantStatusHandler builds the shared handler body ativar/suspender
// use, parameterized only by the fixed status literal each mounts --
// mirroring internal/modules/ordering's own ativar/desativar handler pair.
func setTenantStatusHandler(store tenantAdminStore, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		tenant, err := store.SetTenantStatus(r.Context(), id, status)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newTenantResponse(tenant), nil)
	}
}
