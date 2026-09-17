package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	platformadminhttp "github.com/manuellysuzik/mesaposta-api/internal/platform/platformadmin/http"
)

func newTestRouter(store *fakeTenantAdminStore) chi.Router {
	r := chi.NewRouter()
	platformadminhttp.Mount(r, store)
	return r
}

func doRequest(r chi.Router, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
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

func TestCreateTenantHandler_ValidRequest_Returns201(t *testing.T) {
	r := newTestRouter(newFakeTenantAdminStore())

	w := doRequest(r, http.MethodPost, "/", map[string]any{
		"slug": "acme-burger", "name": "Acme Burger", "plan": "basic",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	var data map[string]any
	if err := json.Unmarshal(env["data"], &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if data["status"] != "trial" {
		t.Errorf("data[status] = %v, want %q", data["status"], "trial")
	}
	if data["is_active"] != true {
		t.Errorf("data[is_active] = %v, want true", data["is_active"])
	}
}

func TestCreateTenantHandler_InvalidSlug_Returns400(t *testing.T) {
	r := newTestRouter(newFakeTenantAdminStore())

	w := doRequest(r, http.MethodPost, "/", map[string]any{
		"slug": "Not A Valid Slug!", "name": "Acme Burger", "plan": "basic",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateTenantHandler_InvalidPlan_Returns400(t *testing.T) {
	r := newTestRouter(newFakeTenantAdminStore())

	w := doRequest(r, http.MethodPost, "/", map[string]any{
		"slug": "acme-burger", "name": "Acme Burger", "plan": "gold-tier",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateTenantHandler_DuplicateSlug_Returns409(t *testing.T) {
	r := newTestRouter(newFakeTenantAdminStore())

	first := doRequest(r, http.MethodPost, "/", map[string]any{
		"slug": "acme-burger", "name": "Acme Burger", "plan": "basic",
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := doRequest(r, http.MethodPost, "/", map[string]any{
		"slug": "acme-burger", "name": "Another Name", "plan": "pro",
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate create: status = %d, want %d (body=%s)", second.Code, http.StatusConflict, second.Body.String())
	}
}

func TestListTenantsHandler_ReturnsCreatedTenants(t *testing.T) {
	store := newFakeTenantAdminStore()
	r := newTestRouter(store)

	doRequest(r, http.MethodPost, "/", map[string]any{"slug": "tenant-a", "name": "Tenant A", "plan": "basic"})
	doRequest(r, http.MethodPost, "/", map[string]any{"slug": "tenant-b", "name": "Tenant B", "plan": "pro"})

	w := doRequest(r, http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body)
	var data []map[string]any
	if err := json.Unmarshal(env["data"], &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if len(data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(data))
	}
}

func TestUpdateTenantDetailsHandler_ValidRequest_Returns200(t *testing.T) {
	store := newFakeTenantAdminStore()
	r := newTestRouter(store)

	created := doRequest(r, http.MethodPost, "/", map[string]any{"slug": "acme-burger", "name": "Old Name", "plan": "basic"})
	env := decodeEnvelope(t, created.Body)
	var data map[string]any
	if err := json.Unmarshal(env["data"], &data); err != nil {
		t.Fatalf("decoding create data: %v", err)
	}
	id := data["id"].(string)

	w := doRequest(r, http.MethodPost, "/"+id+"/atualizar", map[string]any{"name": "New Name", "plan": "enterprise"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	updatedEnv := decodeEnvelope(t, w.Body)
	var updated map[string]any
	if err := json.Unmarshal(updatedEnv["data"], &updated); err != nil {
		t.Fatalf("decoding update data: %v", err)
	}
	if updated["name"] != "New Name" {
		t.Errorf("updated[name] = %v, want %q", updated["name"], "New Name")
	}
	if updated["plan"] != "enterprise" {
		t.Errorf("updated[plan] = %v, want %q", updated["plan"], "enterprise")
	}
	if updated["slug"] != "acme-burger" {
		t.Errorf("updated[slug] = %v, want unchanged %q", updated["slug"], "acme-burger")
	}
}

func TestUpdateTenantDetailsHandler_UnknownID_Returns404(t *testing.T) {
	r := newTestRouter(newFakeTenantAdminStore())

	w := doRequest(r, http.MethodPost, "/00000000-0000-0000-0000-000000000000/atualizar", map[string]any{
		"name": "Doesn't Matter", "plan": "basic",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestActivateTenantHandler_SetsStatusActiveAndIsActiveTrue(t *testing.T) {
	store := newFakeTenantAdminStore()
	r := newTestRouter(store)

	created := doRequest(r, http.MethodPost, "/", map[string]any{"slug": "acme-burger", "name": "Acme", "plan": "basic"})
	env := decodeEnvelope(t, created.Body)
	var data map[string]any
	json.Unmarshal(env["data"], &data)
	id := data["id"].(string)

	doRequest(r, http.MethodPost, "/"+id+"/suspender", nil)

	w := doRequest(r, http.MethodPost, "/"+id+"/ativar", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	activatedEnv := decodeEnvelope(t, w.Body)
	var activated map[string]any
	json.Unmarshal(activatedEnv["data"], &activated)
	if activated["status"] != "active" {
		t.Errorf("activated[status] = %v, want %q", activated["status"], "active")
	}
	if activated["is_active"] != true {
		t.Errorf("activated[is_active] = %v, want true", activated["is_active"])
	}
}

func TestSuspendTenantHandler_SetsStatusSuspendedAndIsActiveFalse(t *testing.T) {
	store := newFakeTenantAdminStore()
	r := newTestRouter(store)

	created := doRequest(r, http.MethodPost, "/", map[string]any{"slug": "acme-burger", "name": "Acme", "plan": "basic"})
	env := decodeEnvelope(t, created.Body)
	var data map[string]any
	json.Unmarshal(env["data"], &data)
	id := data["id"].(string)

	w := doRequest(r, http.MethodPost, "/"+id+"/suspender", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	suspendedEnv := decodeEnvelope(t, w.Body)
	var suspended map[string]any
	json.Unmarshal(suspendedEnv["data"], &suspended)
	if suspended["status"] != "suspended" {
		t.Errorf("suspended[status] = %v, want %q", suspended["status"], "suspended")
	}
	if suspended["is_active"] != false {
		t.Errorf("suspended[is_active] = %v, want false", suspended["is_active"])
	}
}
