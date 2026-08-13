package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// listOrdersHandler returns a handler for GET /pedidos.
func listOrdersHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		orders, err := application.ListOrders(r.Context(), uow)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		out := make([]orderResponse, len(orders))
		for i, order := range orders {
			resp, err := newOrderResponse(order)
			if err != nil {
				httpx.RespondError(w, r, err)
				return
			}
			out[i] = resp
		}
		httpx.RespondData(w, r, http.StatusOK, out, nil)
	}
}

// getOrderHandler returns a handler for GET /pedidos/{id}.
func getOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		order, err := application.GetOrder(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		resp, err := newOrderResponse(order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, resp, nil)
	}
}

// orderTransitionHandler builds a handler that loads the {id} path param
// and runs transition against it, responding with the resulting order --
// every one of this module's status-advancing routes (confirmar, preparar,
// pronto, entregar, cancelar) is this exact shape with a different
// application-layer transition function, so it is factored out once here.
func orderTransitionHandler(transition func(r *http.Request, id domain.ID) (domain.Order, error)) http.HandlerFunc {
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

		order, err := transition(r, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		resp, err := newOrderResponse(&order)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, resp, nil)
	}
}

// confirmOrderHandler returns a handler for POST /pedidos/{id}/confirmar.
func confirmOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return orderTransitionHandler(func(r *http.Request, id domain.ID) (domain.Order, error) {
		return application.ConfirmOrder(r.Context(), uow, id)
	})
}

// startPreparingOrderHandler returns a handler for
// POST /pedidos/{id}/preparar.
func startPreparingOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return orderTransitionHandler(func(r *http.Request, id domain.ID) (domain.Order, error) {
		return application.StartPreparingOrder(r.Context(), uow, id)
	})
}

// markOrderReadyHandler returns a handler for POST /pedidos/{id}/pronto.
func markOrderReadyHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return orderTransitionHandler(func(r *http.Request, id domain.ID) (domain.Order, error) {
		return application.MarkOrderReady(r.Context(), uow, id)
	})
}

// markOrderDeliveredHandler returns a handler for
// POST /pedidos/{id}/entregar.
func markOrderDeliveredHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return orderTransitionHandler(func(r *http.Request, id domain.ID) (domain.Order, error) {
		return application.MarkOrderDelivered(r.Context(), uow, id)
	})
}

// cancelOrderHandler returns a handler for POST /pedidos/{id}/cancelar.
func cancelOrderHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return orderTransitionHandler(func(r *http.Request, id domain.ID) (domain.Order, error) {
		return application.CancelOrder(r.Context(), uow, id)
	})
}
