package http

import (
	"net/http"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func getPurchaseOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		order, err := application.NewGetPurchaseOrder(uow).Execute(r.Context(), tenantID, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		if order == nil {
			httpx.RespondError(w, r, httpx.NotFoundError("purchase_order_not_found", "purchase order not found", nil))
			return
		}
		resp, err := newPurchaseOrderResponse(order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, resp, nil)
	}
}

func createStockItemHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		req, err := decodeJSON[createStockItemRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		unit, err := domain.ParseStockUnit(req.Unit)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		qty, err := domain.NewQuantity(req.QuantityThousandths)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		minQty, err := domain.NewQuantity(req.MinQuantityThousandths)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		cost, err := domain.NewMoney(req.CostPerUnitCents)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		var supplierID *domain.ID
		if req.SupplierID != nil {
			sid, err := domain.ParseID(*req.SupplierID)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			supplierID = &sid
		}
		item, err := application.NewCreateStockItem(uow).Execute(r.Context(), tenantID, req.Name, unit, qty, minQty, cost, supplierID)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, newStockItemResponse(item), nil)
	}
}

func createPurchaseOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		req, err := decodeJSON[createPurchaseOrderRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		supplierID, err := domain.ParseID(req.SupplierID)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		items := make([]domain.PurchaseOrderItem, len(req.Items))
		for i, line := range req.Items {
			stockID, err := domain.ParseID(line.StockItemID)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			qty, err := domain.NewQuantity(line.QuantityThousandths)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			cost, err := domain.NewMoney(line.CostPerUnitCents)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			items[i] = domain.PurchaseOrderItem{StockItemID: stockID, Quantity: qty, CostPerUnit: cost}
		}
		order, err := application.NewCreatePurchaseOrder(uow).Execute(r.Context(), tenantID, supplierID, items)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		resp, err := newPurchaseOrderResponse(order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, resp, nil)
	}
}

func registerStockMovementHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		req, err := decodeJSON[registerStockMovementRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		movType, err := domain.ParseStockMovementType(req.MovementType)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		qty, err := domain.NewQuantity(req.QuantityThousandths)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		_, err = application.NewRegisterStockMovement(uow).Execute(r.Context(), tenantID, id, movType, qty, req.Reason, time.Now().UTC())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		item, _ := application.NewGetStockItem(uow).Execute(r.Context(), tenantID, id)
		if item == nil {
			httpx.RespondError(w, r, httpx.NotFoundError("stock_item_not_found", "stock item not found", nil))
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, newStockItemResponse(item), nil)
	}
}

func receivePurchaseOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		_, err = application.NewReceivePurchaseOrder(uow).Execute(r.Context(), tenantID, id, time.Now().UTC())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		order, _ := application.NewGetPurchaseOrder(uow).Execute(r.Context(), tenantID, id)
		if order == nil {
			httpx.RespondError(w, r, httpx.NotFoundError("purchase_order_not_found", "purchase order not found", nil))
			return
		}
		resp, err := newPurchaseOrderResponse(order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, resp, nil)
	}
}
