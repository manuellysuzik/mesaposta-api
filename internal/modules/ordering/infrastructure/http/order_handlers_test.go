package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func mustHTTPOrder(t *testing.T, tenantID tenant.TenantID, tableID domain.ID) *domain.Order {
	t.Helper()
	quantity, err := domain.NewQuantity(2)
	if err != nil {
		t.Fatalf("NewQuantity: %v", err)
	}
	price, err := domain.NewMoney(1000)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	item, err := domain.NewOrderItem("p1", quantity, price)
	if err != nil {
		t.Fatalf("NewOrderItem: %v", err)
	}
	order, err := domain.NewOrder(domain.NewID(), tenantID, tableID, nil, nil, []domain.OrderItem{item})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return &order
}

// --- GET /pedidos, /pedidos/{id} -------------------------------------------

func TestListOrdersHandler_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.tables[table.ID()] = table
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/pedidos/", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetOrderHandler_NotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/pedidos/"+domain.NewID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- Status transitions -----------------------------------------------------

func TestConfirmOrderHandler_Valid_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.tables[table.ID()] = table
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/confirmar", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got := uow.snapshot().orders[order.ID()].Status(); got != domain.OrderStatusConfirmado {
		t.Errorf("persisted status = %q, want %q", got, domain.OrderStatusConfirmado)
	}
}

// TestStartPreparingOrderHandler_BeforeConfirm_Returns409 proves an
// invalid state transition surfaces as HTTP 409, not 400/500, mirroring
// this codebase's own established pattern for optimistic-concurrency/state
// conflicts (e.g. crm's redeem-loyalty-points 409 test).
func TestStartPreparingOrderHandler_BeforeConfirm_Returns409(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.tables[table.ID()] = table
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/preparar", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestCancelOrderHandler_Valid_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.tables[table.ID()] = table
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/cancelar", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got := uow.snapshot().orders[order.ID()].Status(); got != domain.OrderStatusCancelado {
		t.Errorf("persisted status = %q, want %q", got, domain.OrderStatusCancelado)
	}
}
