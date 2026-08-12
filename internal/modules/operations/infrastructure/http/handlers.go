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
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		order, err := application.GetPurchaseOrder(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
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
		item, err := application.CreateStockItem(r.Context(), uow, application.CreateStockItemInput{
			TenantID:    tenantID,
			Name:        req.Name,
			Unit:        unit,
			Quantity:    qty,
			MinQuantity: minQty,
			CostPerUnit: cost,
			SupplierID:  supplierID,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, newStockItemResponse(&item), nil)
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
			item, err := domain.NewPurchaseOrderItem(stockID, qty, cost)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			items[i] = item
		}
		order, err := application.CreatePurchaseOrder(r.Context(), uow, application.CreatePurchaseOrderInput{
			TenantID:   tenantID,
			SupplierID: supplierID,
			Items:      items,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		resp, err := newPurchaseOrderResponse(&order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, resp, nil)
	}
}

func registerStockMovementHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
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
		err = application.RegisterStockMovement(r.Context(), uow, application.RegisterStockMovementInput{
			StockItemID:  id,
			MovementType: movType,
			Quantity:     qty,
			Reason:       req.Reason,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		item, err := application.GetStockItem(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, newStockItemResponse(item), nil)
	}
}

func receivePurchaseOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		if err := application.ReceivePurchaseOrder(r.Context(), uow, id, time.Now().UTC()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		order, err := application.GetPurchaseOrder(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
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
