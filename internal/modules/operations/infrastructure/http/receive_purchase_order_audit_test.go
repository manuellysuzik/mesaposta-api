package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	operationshttp "github.com/manuellysuzik/mesaposta-api/internal/modules/operations/infrastructure/http"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file covers ONE thing the rest of this package's handler tests
// cannot: that the "who" recorded in the audit trail is really the
// authenticated caller, all the way from the Authorization header to the
// audit.Entry.
//
// It exercises the real auth.Authenticate middleware with a real signed
// JWT rather than injecting claims directly, for the same reason
// internal/modules/community's own handler tests do: there is no exported
// way to put token.Claims into a context from outside
// internal/platform/auth (auth.withClaims is unexported), so a genuine
// token is the only way to reach that code path from here. That
// encapsulation is deliberate -- a test that could forge claims would also
// be proving nothing about the path production requests actually take.

const (
	auditTestJWTSecret = "test-secret-at-least-32-bytes-long-for-hs256"
	auditTestIssuer    = "mesaposta-api-test"
	auditTestAudience  = "mesaposta-api-test"
	auditTestKID       = "test-kid"
	auditTestTenantID  = "550e8400-e29b-41d4-a716-446655440000"
	auditTestSubject   = "user-42"
)

// newAuthenticatedTestRouter mounts the operations module behind the real
// auth.Authenticate middleware, mirroring how internal/platform/app wires
// it in production.
func newAuthenticatedTestRouter(t *testing.T, uow domain.UnitOfWork) chi.Router {
	t.Helper()
	verifier, err := token.NewVerifier(
		map[string][]byte{auditTestKID: []byte(auditTestJWTSecret)}, auditTestIssuer, auditTestAudience)
	if err != nil {
		t.Fatalf("token.NewVerifier: %v", err)
	}

	r := chi.NewRouter()
	r.Use(auth.Authenticate(verifier))
	operationshttp.Mount(r, uow)
	return r
}

// mustAuditTestAccessToken mints a signed access token whose "sub" claim is
// subject and whose tenant_id claim is auditTestTenantID.
func mustAuditTestAccessToken(t *testing.T, subject string) string {
	t.Helper()
	issuer, err := token.NewIssuer(auditTestKID, []byte(auditTestJWTSecret), auditTestIssuer, auditTestAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	tok, err := issuer.Issue(subject, auditTestTenantID, token.RoleGestor, "session-"+subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

// receivableOrderFixture seeds fake with one pending PurchaseOrder for a
// single StockItem, wired so that a receive request against it succeeds.
func receivableOrderFixture(t *testing.T, fake *fakeUnitOfWork) *domain.PurchaseOrder {
	t.Helper()

	tenantID, err := tenant.Parse(auditTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}

	qty, err := domain.NewQuantity(2000)
	if err != nil {
		t.Fatalf("NewQuantity: %v", err)
	}
	zeroQty, err := domain.NewQuantity(0)
	if err != nil {
		t.Fatalf("NewQuantity(0): %v", err)
	}
	cost, err := domain.NewMoney(500)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}

	item, err := domain.NewStockItem(
		domain.NewID(), tenantID, "Farinha de trigo", domain.StockUnitKg, zeroQty, zeroQty, cost, nil)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}

	poItem, err := domain.NewPurchaseOrderItem(item.ID(), qty, cost)
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem: %v", err)
	}
	order, err := domain.NewPurchaseOrder(domain.NewID(), tenantID, domain.NewID(), []domain.PurchaseOrderItem{poItem})
	if err != nil {
		t.Fatalf("NewPurchaseOrder: %v", err)
	}

	fake.repos.purchaseOrders.findByIDForUpdate = func(_ context.Context, _ domain.ID) (*domain.PurchaseOrder, error) {
		return &order, nil
	}
	fake.repos.purchaseOrders.findByID = func(_ context.Context, _ domain.ID) (*domain.PurchaseOrder, error) {
		return &order, nil
	}
	fake.repos.stockItems.findManyByIDs = func(_ context.Context, _ []domain.ID) (map[domain.ID]*domain.StockItem, error) {
		return map[domain.ID]*domain.StockItem{item.ID(): &item}, nil
	}

	return &order
}

func TestReceivePurchaseOrderHandler_RecordsAuthenticatedSubjectAsAuditActor(t *testing.T) {
	// Arrange
	fake := newFakeUnitOfWork()
	order := receivableOrderFixture(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/purchase-orders/"+order.ID().String()+"/receive", nil)
	req.Header.Set("Authorization", "Bearer "+mustAuditTestAccessToken(t, auditTestSubject))
	w := httptest.NewRecorder()

	// Act
	newAuthenticatedTestRouter(t, fake).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	recorded := fake.repos.auditEntries.recorded
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	entry := recorded[0]
	if entry.ActorID != auditTestSubject {
		t.Errorf("entry.ActorID = %q, want %q (the JWT \"sub\" claim)", entry.ActorID, auditTestSubject)
	}
	if entry.Action != "purchase_order.received" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "purchase_order.received")
	}
	if entry.EntityID != order.ID().String() {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, order.ID().String())
	}
}

// TestReceivePurchaseOrderHandler_NoClaims_RecordsEmptyActorAndStillSucceeds
// pins down the "audit must never block the operation" rule: a request
// carrying a tenant but no claims (the shape the non-authenticated test
// router in this package produces, and the shape an internal caller would
// have) still receives the purchase order -- it just records no actor,
// rather than failing or inventing one.
func TestReceivePurchaseOrderHandler_NoClaims_RecordsEmptyActorAndStillSucceeds(t *testing.T) {
	// Arrange
	fake := newFakeUnitOfWork()
	order := receivableOrderFixture(t, fake)

	tenantID, err := tenant.Parse(auditTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tenantID)
	req := httptest.NewRequest(http.MethodPost, "/purchase-orders/"+order.ID().String()+"/receive", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	// Act -- newTestRouter mounts the module WITHOUT auth.Authenticate, so
	// the request context carries a tenant but no token.Claims.
	newTestRouter(fake).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	recorded := fake.repos.auditEntries.recorded
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	if got := recorded[0].ActorID; got != "" {
		t.Errorf("entry.ActorID = %q, want %q (no claims means no actor, not a placeholder)", got, "")
	}
}
