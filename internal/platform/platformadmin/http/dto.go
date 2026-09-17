package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// validPlans is the closed set of billing plans a tenant may be created or
// updated with. Duplicated here rather than shared with a domain-level
// enum (mirroring internal/modules/identity/domain.Role's own "each
// boundary owns its closed set" convention): platformadmin has no domain
// layer of its own to own this, and plan has no business meaning to any
// tenant-scoped module.
var validPlans = map[string]bool{
	"basic":      true,
	"pro":        true,
	"enterprise": true,
}

func isValidPlan(s string) bool { return validPlans[s] }

// createTenantRequest is the request payload for POST /v1/platform/tenants.
// Status is deliberately absent: a new tenant always starts in "trial" --
// see db.CreateTenant's own doc comment.
type createTenantRequest struct {
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	OwnerName  *string `json:"owner_name,omitempty"`
	OwnerEmail *string `json:"owner_email,omitempty"`
	Phone      *string `json:"phone,omitempty"`
	Plan       string  `json:"plan"`
}

// updateTenantDetailsRequest is the request payload for POST
// /v1/platform/tenants/{id}/atualizar. Slug is deliberately absent -- see
// db.UpdateTenantDetailsInput's own doc comment for why it is immutable
// after creation. Every other field is a full replace, not a partial
// patch -- see that same doc comment: omitting OwnerName/OwnerEmail/Phone
// clears them, it does not leave them unchanged.
type updateTenantDetailsRequest struct {
	Name       string  `json:"name"`
	OwnerName  *string `json:"owner_name,omitempty"`
	OwnerEmail *string `json:"owner_email,omitempty"`
	Phone      *string `json:"phone,omitempty"`
	Plan       string  `json:"plan"`
}

// tenantResponse is the JSON shape every handler in this package returns
// for a single tenant.
type tenantResponse struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	OwnerName  *string `json:"owner_name,omitempty"`
	OwnerEmail *string `json:"owner_email,omitempty"`
	Phone      *string `json:"phone,omitempty"`
	Plan       string  `json:"plan"`
	Status     string  `json:"status"`
	IsActive   bool    `json:"is_active"`
	CreatedAt  string  `json:"created_at"`
}

// newTenantResponse converts a db.PlatformTenant into the wire response
// shape -- the one place in this package allowed to know about
// internal/platform/db's raw row type.
func newTenantResponse(t db.PlatformTenant) tenantResponse {
	return tenantResponse{
		ID:         t.ID,
		Slug:       t.Slug,
		Name:       t.Name,
		OwnerName:  t.OwnerName,
		OwnerEmail: t.OwnerEmail,
		Phone:      t.Phone,
		Plan:       t.Plan,
		Status:     t.Status,
		IsActive:   t.IsActive,
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
	}
}
