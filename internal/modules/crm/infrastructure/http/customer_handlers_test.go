package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	crmhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const testTenantIDStr = "cccccccc-dddd-eeee-ffff-000000000000"

func mustHTTPTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(testTenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", testTenantIDStr, err)
	}
	return id
}

// newTestRouter builds a chi.Router with the crm module mounted against
// uow, ready for httptest-based handler tests.
func newTestRouter(uow *fakeUnitOfWork) chi.Router {
	r := chi.NewRouter()
	crmhttp.Mount(r, uow)
	return r
}

// doRequest issues req against r, optionally carrying an authenticated
// tenant (withTenant == true), and returns the recorded response.
func doRequest(r chi.Router, req *http.Request, withTenant bool, tenantID tenant.TenantID) *httptest.ResponseRecorder {
	if withTenant {
		req = req.WithContext(tenant.With(req.Context(), tenantID))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeEnvelope(t *testing.T, body *bytes.Buffer) map[string]json.RawMessage {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response envelope: %v (body=%s)", err, body.String())
	}
	return envelope
}

func mustHTTPCustomer(t *testing.T, tenantID tenant.TenantID) *domain.Customer {
	t.Helper()
	customer, err := domain.NewCustomer(domain.NewID(), tenantID, "Cliente HTTP", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer: %v", err)
	}
	return &customer
}

// --- GET /customers/{id} ---------------------------------------------------

func TestGetCustomerHandler_Found_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/customers/"+customer.ID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	envelope := decodeEnvelope(t, w.Body)
	if envelope["error"] != nil && string(envelope["error"]) != "null" {
		t.Errorf("envelope.error = %s, want null", envelope["error"])
	}
}

func TestGetCustomerHandler_NotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/customers/"+domain.NewID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetCustomerHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/customers/not-a-uuid", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetCustomerHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/customers/"+domain.NewID().String(), nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /customers --------------------------------------------------------

func TestCreateCustomerHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "Novo Cliente"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.customers) != 1 {
		t.Errorf("customers persisted = %d, want 1", len(snapshot.customers))
	}
}

func TestCreateCustomerHandler_InvalidBody_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": ""})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateCustomerHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "Cliente"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /customers/{id}/loyalty/credit -------------------------------------

func TestCreditLoyaltyPointsHandler_Valid_Returns200AndUpdatesBalance(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"points": 50, "reason": "Pedido #1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/credit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	persisted := uow.snapshot().customers[customer.ID()]
	if persisted.LoyaltyPoints().Points() != 50 {
		t.Errorf("persisted.LoyaltyPoints().Points() = %d, want 50", persisted.LoyaltyPoints().Points())
	}
}

func TestCreditLoyaltyPointsHandler_CustomerNotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"points": 50, "reason": "Pedido #1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+domain.NewID().String()+"/loyalty/credit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- POST /customers/{id}/loyalty/redeem -------------------------------------

// TestRedeemLoyaltyPointsHandler_InsufficientBalance_Returns409 is this
// module's headline acceptance criterion, proven once more at the HTTP
// boundary: a redemption exceeding the customer's balance must surface as
// HTTP 409, not 400/500, and must leave the persisted balance untouched.
func TestRedeemLoyaltyPointsHandler_InsufficientBalance_Returns409(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	if err := customer.CreditPoints(mustFakeHTTPLoyaltyPoints(t, 20)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"points": 50, "reason": "Resgate caro demais"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	persisted := uow.snapshot().customers[customer.ID()]
	if persisted.LoyaltyPoints().Points() != 20 {
		t.Errorf("persisted.LoyaltyPoints().Points() = %d after rejected redeem, want unchanged 20", persisted.LoyaltyPoints().Points())
	}
}

func TestRedeemLoyaltyPointsHandler_Valid_Returns200AndUpdatesBalance(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	if err := customer.CreditPoints(mustFakeHTTPLoyaltyPoints(t, 100)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"points": 30, "reason": "Resgate de brinde"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	persisted := uow.snapshot().customers[customer.ID()]
	if persisted.LoyaltyPoints().Points() != 70 {
		t.Errorf("persisted.LoyaltyPoints().Points() = %d, want 70", persisted.LoyaltyPoints().Points())
	}
}

func mustFakeHTTPLoyaltyPoints(t *testing.T, points int64) domain.LoyaltyPoints {
	t.Helper()
	p, err := domain.NewLoyaltyPoints(points)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(%d): %v", points, err)
	}
	return p
}
