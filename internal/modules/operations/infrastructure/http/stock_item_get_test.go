package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	operationshttp "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/http"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func newTestRouter(uow domain.UnitOfWork) chi.Router {
	r := chi.NewRouter()
	operationshttp.Mount(r, uow)
	return r
}

func TestGetStockItemHandler_Success(t *testing.T) {
	tenantID, _ := tenant.Parse("550e8400-e29b-41d4-a716-446655440000")
	itemID, _ := domain.ParseID("550e8400-e29b-41d4-a716-446655440001")
	qty, _ := domain.NewQuantity(1000)
	minQty, _ := domain.NewQuantity(100)
	cost, _ := domain.NewMoney(2500)
	item, err := domain.NewStockItem(itemID, tenantID, "test", domain.StockUnitUn, qty, minQty, cost, nil)
	if err != nil {
		t.Fatalf("failed to build test fixture: %v", err)
	}

	fake := newFakeUnitOfWork()
	fake.repos.stockItems.findByID = func(ctx context.Context, id domain.ID) (*domain.StockItem, error) {
		if id.String() == itemID.String() {
			return &item, nil
		}
		return nil, errs.New(errs.NotFound, "stock_item_not_found", "stock item not found", nil)
	}

	ctx := tenant.With(context.Background(), tenantID)
	r := httptest.NewRequest(http.MethodGet, "/stock-items/"+itemID.String(), nil).WithContext(ctx)
	w := httptest.NewRecorder()

	newTestRouter(fake).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStockItemHandler_NotFound(t *testing.T) {
	tenantID, _ := tenant.Parse("550e8400-e29b-41d4-a716-446655440000")
	unknownID, _ := domain.ParseID("550e8400-e29b-41d4-a716-446655440099")

	fake := newFakeUnitOfWork()
	fake.repos.stockItems.findByID = func(ctx context.Context, id domain.ID) (*domain.StockItem, error) {
		return nil, errs.New(errs.NotFound, "stock_item_not_found", "stock item not found", nil)
	}

	ctx := tenant.With(context.Background(), tenantID)
	r := httptest.NewRequest(http.MethodGet, "/stock-items/"+unknownID.String(), nil).WithContext(ctx)
	w := httptest.NewRecorder()

	newTestRouter(fake).ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStockItemHandler_MalformedID(t *testing.T) {
	tenantID, _ := tenant.Parse("550e8400-e29b-41d4-a716-446655440000")
	fake := newFakeUnitOfWork()

	ctx := tenant.With(context.Background(), tenantID)
	r := httptest.NewRequest(http.MethodGet, "/stock-items/not-a-uuid", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	newTestRouter(fake).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStockItemHandler_MissingTenant(t *testing.T) {
	fake := newFakeUnitOfWork()

	r := httptest.NewRequest(http.MethodGet, "/stock-items/550e8400-e29b-41d4-a716-446655440001", nil)
	w := httptest.NewRecorder()

	newTestRouter(fake).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}
