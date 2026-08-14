package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// getCustomerHandler returns a handler for GET /v1/customers/{id}.
func getCustomerHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		customer, err := application.GetCustomer(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newCustomerResponse(customer), nil)
	}
}

// createCustomerHandler returns a handler for POST /v1/customers.
func createCustomerHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createCustomerRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		customer, err := application.CreateCustomer(r.Context(), uow, application.CreateCustomerInput{
			TenantID: tenantID,
			Name:     req.Name,
			Email:    req.Email,
			Phone:    req.Phone,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newCustomerResponse(&customer), nil)
	}
}

// creditLoyaltyPointsHandler returns a handler for
// POST /v1/customers/{id}/loyalty/credit.
func creditLoyaltyPointsHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		req, err := decodeJSON[loyaltyPointsRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		points, err := domain.NewLoyaltyPoints(req.Points)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := application.CreditLoyaltyPoints(r.Context(), uow, application.CreditLoyaltyPointsInput{
			CustomerID: id,
			Points:     points,
			Reason:     req.Reason,
		}, auth.ActorIDFrom(r.Context())); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		customer, err := application.GetCustomer(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newCustomerResponse(customer), nil)
	}
}

// redeemLoyaltyPointsHandler returns a handler for
// POST /v1/customers/{id}/loyalty/redeem.
//
// The critical response shape this handler is responsible for: when
// application.RedeemLoyaltyPoints rejects a redemption that exceeds the
// customer's balance (Kind errs.Conflict), httpx.RespondError maps that to
// HTTP 409 -- see internal/platform/httpx/errormap.go's kindStatus table --
// with nothing persisted, per Customer.RedeemPoints' own doc comment.
func redeemLoyaltyPointsHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		req, err := decodeJSON[loyaltyPointsRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		points, err := domain.NewLoyaltyPoints(req.Points)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := application.RedeemLoyaltyPoints(r.Context(), uow, application.RedeemLoyaltyPointsInput{
			CustomerID: id,
			Points:     points,
			Reason:     req.Reason,
		}, auth.ActorIDFrom(r.Context())); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		customer, err := application.GetCustomer(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newCustomerResponse(customer), nil)
	}
}
