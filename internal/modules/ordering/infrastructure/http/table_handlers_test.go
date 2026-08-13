package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	orderinghttp "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
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

// newTestRouter builds a chi.Router with both of ordering's authenticated
// route groups mounted against uow, ready for httptest-based handler
// tests. Since MountTables/MountOrders apply NO role restriction
// themselves (see router.go's own doc comment), this test router does not
// need to fabricate any auth.RequireRole-satisfying claims -- only a
// tenant, exactly like every other module's own handler test suite in this
// codebase.
func newTestRouter(uow *fakeUnitOfWork) chi.Router {
	r := chi.NewRouter()
	orderinghttp.MountTables(r, uow)
	orderinghttp.MountOrders(r, uow)
	return r
}

func doRequest(r chi.Router, req *http.Request, withTenant bool, tenantID tenant.TenantID) *httptest.ResponseRecorder {
	if withTenant {
		req = req.WithContext(tenant.With(req.Context(), tenantID))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func mustHTTPTable(t *testing.T, tenantID tenant.TenantID) *domain.Table {
	t.Helper()
	table, err := domain.NewTable(domain.NewID(), tenantID, "M01", "Mesa 1")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	return &table
}

// --- GET /mesas --------------------------------------------------------

func TestListTablesHandler_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/mesas/", nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestListTablesHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/mesas/", nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /mesas --------------------------------------------------------

func TestCreateTableHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, _ := json.Marshal(map[string]any{"code": "M01", "label": "Mesa 1"})
	req := httptest.NewRequest(http.MethodPost, "/mesas/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.tables) != 1 {
		t.Errorf("tables persisted = %d, want 1", len(snapshot.tables))
	}
}

func TestCreateTableHandler_InvalidBody_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, _ := json.Marshal(map[string]any{"code": "", "label": "Mesa 1"})
	req := httptest.NewRequest(http.MethodPost, "/mesas/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// --- GET /mesas/{id} ------------------------------------------------------

func TestGetTableHandler_NotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/mesas/"+domain.NewID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- POST /mesas/{id}/ativar, /desativar ------------------------------------

func TestActivateDeactivateTableHandlers_RoundTrip(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	state := newFakeState()
	state.tables[table.ID()] = table
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPost, "/mesas/"+table.ID().String()+"/desativar", nil)
	w := doRequest(r, req, true, tenantID)
	if w.Code != http.StatusOK {
		t.Fatalf("desativar: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if uow.snapshot().tables[table.ID()].IsActive() {
		t.Error("table still active after desativar")
	}

	req = httptest.NewRequest(http.MethodPost, "/mesas/"+table.ID().String()+"/ativar", nil)
	w = doRequest(r, req, true, tenantID)
	if w.Code != http.StatusOK {
		t.Fatalf("ativar: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !uow.snapshot().tables[table.ID()].IsActive() {
		t.Error("table still inactive after ativar")
	}
}
