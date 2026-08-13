package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// tableResponse is the JSON representation of a Table aggregate.
type tableResponse struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	IsActive  bool      `json:"is_active"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newTableResponse(t *domain.Table) tableResponse {
	return tableResponse{
		ID:        t.ID().String(),
		TenantID:  t.TenantID().String(),
		Code:      t.Code(),
		Label:     t.Label(),
		IsActive:  t.IsActive(),
		Version:   t.Version(),
		CreatedAt: t.CreatedAt(),
		UpdatedAt: t.UpdatedAt(),
	}
}

// createTableRequest is the request payload for POST /mesas.
type createTableRequest struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// orderItemResponse is the JSON representation of a single OrderItem line.
type orderItemResponse struct {
	ProductID      string `json:"product_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

// orderResponse is the JSON representation of an Order aggregate,
// including its computed total.
type orderResponse struct {
	ID            string              `json:"id"`
	TenantID      string              `json:"tenant_id"`
	TableID       string              `json:"table_id"`
	CustomerName  *string             `json:"customer_name"`
	CustomerPhone *string             `json:"customer_phone"`
	Items         []orderItemResponse `json:"items"`
	TotalCents    int64               `json:"total_cents"`
	Status        string              `json:"status"`
	Version       int64               `json:"version"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

func newOrderResponse(o *domain.Order) (orderResponse, error) {
	items := o.Items()
	itemResponses := make([]orderItemResponse, len(items))
	for i, item := range items {
		itemResponses[i] = orderItemResponse{
			ProductID:      item.ProductID(),
			Quantity:       item.Quantity().Count(),
			UnitPriceCents: item.UnitPrice().Cents(),
		}
	}

	total, err := o.Total()
	if err != nil {
		return orderResponse{}, err
	}

	return orderResponse{
		ID:            o.ID().String(),
		TenantID:      o.TenantID().String(),
		TableID:       o.TableID().String(),
		CustomerName:  o.CustomerName(),
		CustomerPhone: o.CustomerPhone(),
		Items:         itemResponses,
		TotalCents:    total.Cents(),
		Status:        string(o.Status()),
		Version:       o.Version(),
		CreatedAt:     o.CreatedAt(),
		UpdatedAt:     o.UpdatedAt(),
	}, nil
}
