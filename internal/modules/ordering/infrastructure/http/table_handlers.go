package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// listTablesHandler returns a handler for GET /mesas.
func listTablesHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		tables, err := application.ListTables(r.Context(), uow)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		out := make([]tableResponse, len(tables))
		for i, table := range tables {
			out[i] = newTableResponse(table)
		}
		httpx.RespondData(w, r, http.StatusOK, out, nil)
	}
}

// getTableHandler returns a handler for GET /mesas/{id}.
func getTableHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		table, err := application.GetTable(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, newTableResponse(table), nil)
	}
}

// createTableHandler returns a handler for POST /mesas.
func createTableHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createTableRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		table, err := application.CreateTable(r.Context(), uow, application.CreateTableInput{
			TenantID: tenantID,
			Code:     req.Code,
			Label:    req.Label,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusCreated, newTableResponse(&table), nil)
	}
}

// activateTableHandler returns a handler for POST /mesas/{id}/ativar.
func activateTableHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		table, err := application.ActivateTable(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, newTableResponse(&table), nil)
	}
}

// deactivateTableHandler returns a handler for POST /mesas/{id}/desativar.
func deactivateTableHandler(uow domain.UnitOfWork) http.HandlerFunc {
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

		table, err := application.DeactivateTable(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		httpx.RespondData(w, r, http.StatusOK, newTableResponse(&table), nil)
	}
}
