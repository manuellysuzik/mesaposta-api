package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file rounds out this package's handler test suite with the
// remaining request-shape/error-path cases not already covered by
// table_handlers_test.go/order_handlers_test.go -- malformed path
// params, missing-tenant on every remaining handler, and the full
// preparar->pronto->entregar tail of the order lifecycle.

func TestGetTableHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/mesas/not-a-uuid", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetTableHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/mesas/"+domain.NewID().String(), nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestCreateTableHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/mesas/", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestActivateTableHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/mesas/"+domain.NewID().String()+"/ativar", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestDeactivateTableHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/mesas/not-a-uuid/desativar", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetOrderHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/pedidos/not-a-uuid", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetOrderHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/pedidos/"+domain.NewID().String(), nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestListOrdersHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/pedidos/", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestConfirmOrderHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/not-a-uuid/confirmar", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestConfirmOrderHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+domain.NewID().String()+"/confirmar", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// TestOrderLifecycleHandlers_FullChain walks confirmar -> preparar ->
// pronto -> entregar via real HTTP requests against this package's own
// router, exercising markOrderReadyHandler/markOrderDeliveredHandler's
// happy paths (not otherwise hit by order_handlers_test.go, which only
// proves the invalid-transition 409 case for "preparar").
func TestOrderLifecycleHandlers_FullChain(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.tables[table.ID()] = table
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	steps := []string{"confirmar", "preparar", "pronto", "entregar"}
	for _, step := range steps {
		req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/"+step, nil)
		w := doRequest(r, req, true, tenantID)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d (body=%s)", step, w.Code, http.StatusOK, w.Body.String())
		}
	}

	final := uow.snapshot().orders[order.ID()]
	if final.Status() != domain.OrderStatusEntregue {
		t.Errorf("final status = %q, want %q", final.Status(), domain.OrderStatusEntregue)
	}
}

func TestCancelOrderHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+domain.NewID().String()+"/cancelar", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}
