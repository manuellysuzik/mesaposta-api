package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// stockItemResponse is the JSON representation of a StockItem aggregate.
type stockItemResponse struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	Name                   string    `json:"name"`
	Unit                   string    `json:"unit"`
	QuantityThousandths    int64     `json:"quantity_thousandths"`
	MinQuantityThousandths int64     `json:"min_quantity_thousandths"`
	CostPerUnitCents       int64     `json:"cost_per_unit_cents"`
	SupplierID             *string   `json:"supplier_id"`
	Version                int64     `json:"version"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// newStockItemResponse maps a domain StockItem to its HTTP response.
func newStockItemResponse(item *domain.StockItem) stockItemResponse {
	var supplierID *string
	if sid := item.SupplierID(); sid != nil {
		s := sid.String()
		supplierID = &s
	}
	return stockItemResponse{
		ID:                     item.ID().String(),
		TenantID:               item.TenantID().String(),
		Name:                   item.Name(),
		Unit:                   string(item.Unit()),
		QuantityThousandths:    item.Quantity().Thousandths(),
		MinQuantityThousandths: item.MinQuantity().Thousandths(),
		CostPerUnitCents:       item.CostPerUnit().Cents(),
		SupplierID:             supplierID,
		Version:                item.Version(),
		UpdatedAt:              item.UpdatedAt(),
	}
}

// purchaseOrderItemResponse is a line item in a purchase order.
type purchaseOrderItemResponse struct {
	StockItemID         string `json:"stock_item_id"`
	QuantityThousandths int64  `json:"quantity_thousandths"`
	CostPerUnitCents    int64  `json:"cost_per_unit_cents"`
}

// purchaseOrderResponse is the JSON representation of a PurchaseOrder aggregate.
type purchaseOrderResponse struct {
	ID         string                      `json:"id"`
	TenantID   string                      `json:"tenant_id"`
	SupplierID string                      `json:"supplier_id"`
	Status     string                      `json:"status"`
	Items      []purchaseOrderItemResponse `json:"items"`
	TotalCents int64                       `json:"total_cents"`
	ReceivedAt *time.Time                  `json:"received_at"`
	Version    int64                       `json:"version"`
	CreatedAt  time.Time                   `json:"created_at"`
}

// newPurchaseOrderResponse maps a domain PurchaseOrder to its HTTP response.
// Returns an error if Total() fails (overflow).
func newPurchaseOrderResponse(order *domain.PurchaseOrder) (purchaseOrderResponse, error) {
	orderItems := order.Items()
	items := make([]purchaseOrderItemResponse, len(orderItems))
	for i, item := range orderItems {
		items[i] = purchaseOrderItemResponse{
			StockItemID:         item.StockItemID().String(),
			QuantityThousandths: item.Quantity().Thousandths(),
			CostPerUnitCents:    item.CostPerUnit().Cents(),
		}
	}

	total, err := order.Total()
	if err != nil {
		return purchaseOrderResponse{}, errs.New(errs.Internal, "total_calculation_failed", "failed to calculate order total", err)
	}

	return purchaseOrderResponse{
		ID:         order.ID().String(),
		TenantID:   order.TenantID().String(),
		SupplierID: order.SupplierID().String(),
		Status:     string(order.Status()),
		Items:      items,
		TotalCents: total.Cents(),
		ReceivedAt: order.ReceivedAt(),
		Version:    order.Version(),
		CreatedAt:  order.CreatedAt(),
	}, nil
}

// createStockItemRequest is the request payload for POST /v1/stock-items.
type createStockItemRequest struct {
	Name                   string  `json:"name"`
	Unit                   string  `json:"unit"`
	QuantityThousandths    int64   `json:"quantity_thousandths"`
	MinQuantityThousandths int64   `json:"min_quantity_thousandths"`
	CostPerUnitCents       int64   `json:"cost_per_unit_cents"`
	SupplierID             *string `json:"supplier_id"`
}

// createPurchaseOrderRequest is the request payload for POST /v1/purchase-orders.
type createPurchaseOrderRequest struct {
	SupplierID string `json:"supplier_id"`
	Items      []struct {
		StockItemID         string `json:"stock_item_id"`
		QuantityThousandths int64  `json:"quantity_thousandths"`
		CostPerUnitCents    int64  `json:"cost_per_unit_cents"`
	} `json:"items"`
}

// registerStockMovementRequest is the request payload for POST /v1/stock-items/{id}/movements.
type registerStockMovementRequest struct {
	MovementType        string `json:"movement_type"`
	QuantityThousandths int64  `json:"quantity_thousandths"`
	Reason              string `json:"reason"`
}
