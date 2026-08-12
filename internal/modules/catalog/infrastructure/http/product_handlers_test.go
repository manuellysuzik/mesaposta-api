package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	cataloghttp "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
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

// newTestRouter builds a chi.Router with the catalog module mounted
// against uow, ready for httptest-based handler tests.
func newTestRouter(uow *fakeUnitOfWork) chi.Router {
	r := chi.NewRouter()
	cataloghttp.Mount(r, uow)
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

func mustHTTPProduct(t *testing.T, tenantID tenant.TenantID) *domain.Product {
	t.Helper()
	product, err := domain.NewProduct(domain.NewID(), tenantID, "X-Burguer", "Hambúrguer artesanal", mustHTTPMoney(t, 1500), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return &product
}

func mustHTTPMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

// --- GET /products/{id} -----------------------------------------------------

func TestGetProductHandler_Found_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/products/"+product.ID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	envelope := decodeEnvelope(t, w.Body)
	if envelope["error"] != nil && string(envelope["error"]) != "null" {
		t.Errorf("envelope.error = %s, want null", envelope["error"])
	}
}

func TestGetProductHandler_NotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/products/"+domain.NewID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetProductHandler_MalformedID_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/products/not-a-uuid", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetProductHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/products/"+domain.NewID().String(), nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- GET /products (list active) ---------------------------------------------

func TestListActiveProductsHandler_Returns200WithProducts(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/products/", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- POST /products -----------------------------------------------------------

func TestCreateProductHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "X-Burguer", "description": "desc", "base_price_cents": 1500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.products) != 1 {
		t.Errorf("products persisted = %d, want 1", len(snapshot.products))
	}
}

func TestCreateProductHandler_ZeroPrice_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "Refrigerante", "base_price_cents": 0})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateProductHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "X-Burguer", "base_price_cents": 1500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /products/{id}/discount ---------------------------------------------

func TestApplyDiscountHandler_Valid_Returns200AndUpdatesPrice(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"discount_percent_hundredths": 2500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/"+product.ID().String()+"/discount", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	persisted := uow.snapshot().products[product.ID()]
	if persisted.CurrentPrice().Cents() != 1125 {
		t.Errorf("persisted.CurrentPrice().Cents() = %d, want 1125", persisted.CurrentPrice().Cents())
	}
}

// TestApplyDiscountHandler_WouldZeroOutPrice_Returns400 is this module's
// core price invariant, proven once more at the HTTP boundary: a discount
// that would zero out the product's price must surface as HTTP 400, and
// must leave the persisted price untouched.
func TestApplyDiscountHandler_WouldZeroOutPrice_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"discount_percent_hundredths": 10000})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/"+product.ID().String()+"/discount", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	persisted := uow.snapshot().products[product.ID()]
	if persisted.Discount() != nil {
		t.Errorf("persisted.Discount() = %v after rejected discount, want nil", persisted.Discount())
	}
}

func TestApplyDiscountHandler_ProductNotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"discount_percent_hundredths": 1000})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/"+domain.NewID().String()+"/discount", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}
