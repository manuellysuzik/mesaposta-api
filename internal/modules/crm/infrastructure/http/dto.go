package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
)

// customerResponse is the JSON representation of a Customer aggregate.
type customerResponse struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Name          string    `json:"name"`
	Email         *string   `json:"email"`
	Phone         *string   `json:"phone"`
	LoyaltyPoints int64     `json:"loyalty_points"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// newCustomerResponse maps a domain Customer to its HTTP response.
func newCustomerResponse(c *domain.Customer) customerResponse {
	return customerResponse{
		ID:            c.ID().String(),
		TenantID:      c.TenantID().String(),
		Name:          c.Name(),
		Email:         c.Email(),
		Phone:         c.Phone(),
		LoyaltyPoints: c.LoyaltyPoints().Points(),
		Version:       c.Version(),
		CreatedAt:     c.CreatedAt(),
		UpdatedAt:     c.UpdatedAt(),
	}
}

// createCustomerRequest is the request payload for POST /v1/customers.
type createCustomerRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

// loyaltyPointsRequest is the request payload for both
// POST /v1/customers/{id}/loyalty/credit and
// POST /v1/customers/{id}/loyalty/redeem.
type loyaltyPointsRequest struct {
	Points int64  `json:"points"`
	Reason string `json:"reason"`
}
