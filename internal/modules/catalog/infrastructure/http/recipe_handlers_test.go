package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func mustHTTPRecipe(t *testing.T, tenantID tenant.TenantID, ingredients []domain.RecipeIngredient) *domain.Recipe {
	t.Helper()
	recipe, err := domain.NewRecipe(domain.NewID(), tenantID, "Molho especial", "Receita da casa", ingredients)
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}
	return &recipe
}

// --- GET /recipes/{id} -------------------------------------------------------

func TestGetRecipeHandler_Found_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	recipe := mustHTTPRecipe(t, tenantID, nil)
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/recipes/"+recipe.ID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetRecipeHandler_NotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/recipes/"+domain.NewID().String(), nil)
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetRecipeHandler_MissingTenant_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/recipes/"+domain.NewID().String(), nil)
	w := doRequest(r, req, false, tenant.TenantID{})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /recipes -------------------------------------------------------------

func TestCreateRecipeHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": "Molho especial", "description": "desc"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/recipes/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.recipes) != 1 {
		t.Errorf("recipes persisted = %d, want 1", len(snapshot.recipes))
	}
}

func TestCreateRecipeHandler_EmptyName_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"name": ""})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/recipes/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// --- POST /recipes/{id}/promote -------------------------------------------------

func TestPromoteRecipeHandler_WithIngredients_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	ingredient, err := domain.NewRecipeIngredient(domain.NewID(), mustHTTPQuantity(t, 200))
	if err != nil {
		t.Fatalf("NewRecipeIngredient: %v", err)
	}
	recipe := mustHTTPRecipe(t, tenantID, []domain.RecipeIngredient{ingredient})
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"base_price_cents": 2500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/recipes/"+recipe.ID().String()+"/promote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.products) != 1 {
		t.Errorf("products persisted = %d, want 1", len(snapshot.products))
	}
}

// TestPromoteRecipeHandler_NoIngredients_Returns400 is this module's core
// Recipe promotion invariant, proven once more at the HTTP boundary.
func TestPromoteRecipeHandler_NoIngredients_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	recipe := mustHTTPRecipe(t, tenantID, nil)
	state := newFakeState()
	state.recipes[recipe.ID()] = recipe
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"base_price_cents": 1000})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/recipes/"+recipe.ID().String()+"/promote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.products) != 0 {
		t.Errorf("products persisted = %d, want 0", len(snapshot.products))
	}
}

func TestPromoteRecipeHandler_RecipeNotFound_Returns404(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"base_price_cents": 1000})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/recipes/"+domain.NewID().String()+"/promote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := doRequest(r, req, true, tenantID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func mustHTTPQuantity(t *testing.T, thousandths int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(thousandths)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", thousandths, err)
	}
	return q
}
