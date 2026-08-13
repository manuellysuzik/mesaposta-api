package httppublic_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	orderinghttppublic "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/httppublic"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const publicTestTenantIDStr = "dddddddd-eeee-ffff-0000-111111111111"

func mustPublicTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(publicTestTenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	return id
}

// newTestRouter builds a chi.Router with ordering's public routes mounted,
// simulating what internal/platform/auth/publictenant.Middleware would
// have already done (attaching a trusted tenant.TenantID to the request
// context) -- this package's own tests don't re-test that middleware
// (internal/platform/auth/publictenant has its own suite for that), only
// that THIS module's handlers behave correctly given an already-resolved
// tenant.
func newTestRouter(uow *fakeUnitOfWork, menu *fakeMenuReader, price *fakePriceReader) chi.Router {
	r := chi.NewRouter()
	orderinghttppublic.Mount(r, uow, menu, price)
	return r
}

func doPublicRequest(r chi.Router, req *http.Request, tenantID tenant.TenantID) *httptest.ResponseRecorder {
	req = req.WithContext(tenant.With(req.Context(), tenantID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func mustPublicTable(t *testing.T, tenantID tenant.TenantID, code string) *domain.Table {
	t.Helper()
	table, err := domain.NewTable(domain.NewID(), tenantID, code, "Mesa "+code)
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	return &table
}

// --- GET /cardapio/mesa/{code} ---------------------------------------------

func TestGetCardapioHandler_ActiveTable_Returns200WithSessionTokenAndMenu(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	table := mustPublicTable(t, tenantID, "M01")
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)
	menu := &fakeMenuReader{items: []catalogcontracts.MenuItem{
		{ProductID: "p1", Name: "X-Burguer", Description: "", PriceCents: 1500},
	}}
	r := newTestRouter(uow, menu, newFakePriceReader())

	req := httptest.NewRequest(http.MethodGet, "/cardapio/mesa/M01", nil)
	w := doPublicRequest(r, req, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	var envelope struct {
		Data struct {
			SessionToken string `json:"session_token"`
			Items        []struct {
				ProductID string `json:"produto_id"`
			} `json:"itens"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response: %v (body=%s)", err, w.Body.String())
	}
	if envelope.Data.SessionToken == "" {
		t.Error("session_token is empty")
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(envelope.Data.Items))
	}
}

// TestGetCardapioHandler_NonexistentAndInactiveTable_ProduceByteIdenticalResponses
// is this module's single most important security test: an unauthenticated
// caller must NEVER be able to tell "this table code was never issued"
// apart from "this table code exists but the table is currently inactive"
// -- both must yield the exact same status code AND the exact same
// response body, byte for byte.
func TestGetCardapioHandler_NonexistentAndInactiveTable_ProduceByteIdenticalResponses(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	inactiveTable := mustPublicTable(t, tenantID, "M02")
	inactiveTable.Deactivate()

	state := newFakeState()
	state.tables[inactiveTable.ID()] = inactiveTable
	uow := newFakeUnitOfWork(state)
	menu := &fakeMenuReader{}
	r := newTestRouter(uow, menu, newFakePriceReader())

	reqUnknown := httptest.NewRequest(http.MethodGet, "/cardapio/mesa/does-not-exist", nil)
	wUnknown := doPublicRequest(r, reqUnknown, tenantID)

	reqInactive := httptest.NewRequest(http.MethodGet, "/cardapio/mesa/M02", nil)
	wInactive := doPublicRequest(r, reqInactive, tenantID)

	if wUnknown.Code != wInactive.Code {
		t.Fatalf("status differs: unknown=%d inactive=%d", wUnknown.Code, wInactive.Code)
	}
	if wUnknown.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", wUnknown.Code, http.StatusNotFound)
	}

	// Response headers that legitimately vary per-request (X-Request-ID) are
	// excluded before comparing bodies -- the ENVELOPE BODY itself,
	// including its "error" object's code/message, must be byte-identical.
	if !bytes.Equal(wUnknown.Body.Bytes(), wInactive.Body.Bytes()) {
		t.Fatalf("response bodies differ:\nunknown:  %s\ninactive: %s", wUnknown.Body.String(), wInactive.Body.String())
	}
}

func TestGetCardapioHandler_MenuReaderError_Returns500(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	table := mustPublicTable(t, tenantID, "M01")
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)
	menu := &fakeMenuReader{err: errBoom}
	r := newTestRouter(uow, menu, newFakePriceReader())

	req := httptest.NewRequest(http.MethodGet, "/cardapio/mesa/M01", nil)
	w := doPublicRequest(r, req, tenantID)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- POST /pedidos ----------------------------------------------------------

func TestCreatePublicOrderHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	table := mustPublicTable(t, tenantID, "M01")
	session, plaintext, err := domain.NewTableSession(tenantID, table.ID())
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}
	state := newFakeState()
	state.tables[table.ID()] = table
	state.tableSessions[session.TokenHash()] = session
	uow := newFakeUnitOfWork(state)

	priceReader := newFakePriceReader()
	priceReader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 1500, IsActive: true}

	r := newTestRouter(uow, &fakeMenuReader{}, priceReader)

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{{"produto_id": "p1", "quantidade": 2}},
		"nome":  "Diner",
	})
	req := httptest.NewRequest(http.MethodPost, "/pedidos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Table-Session-Token", plaintext)
	w := doPublicRequest(r, req, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if len(uow.snapshot().orders) != 1 {
		t.Error("order was not persisted")
	}
}

func TestCreatePublicOrderHandler_MissingSessionToken_Returns401(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow, &fakeMenuReader{}, newFakePriceReader())

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{{"produto_id": "p1", "quantidade": 1}},
	})
	req := httptest.NewRequest(http.MethodPost, "/pedidos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doPublicRequest(r, req, tenantID)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestCreatePublicOrderHandler_InvalidBody_Returns400(t *testing.T) {
	tenantID := mustPublicTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow, &fakeMenuReader{}, newFakePriceReader())

	req := httptest.NewRequest(http.MethodPost, "/pedidos", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Table-Session-Token", "whatever")
	w := doPublicRequest(r, req, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreatePublicOrderHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow, &fakeMenuReader{}, newFakePriceReader())

	body, _ := json.Marshal(map[string]any{"items": []map[string]any{{"produto_id": "p1", "quantidade": 1}}})
	req := httptest.NewRequest(http.MethodPost, "/pedidos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No tenant attached to context: simulates a request that somehow
	// bypassed publictenant.Middleware.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}
