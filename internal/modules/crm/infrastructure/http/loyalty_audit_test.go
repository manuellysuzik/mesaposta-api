package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	crmhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/crm/infrastructure/http"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file covers ONE thing this package's other handler tests cannot:
// that the "who" recorded in the audit trail is really the authenticated
// caller, all the way from the Authorization header to the audit.Entry --
// mirroring
// internal/modules/operations/infrastructure/http/receive_purchase_order_audit_test.go
// and internal/modules/catalog/infrastructure/http/apply_discount_audit_test.go,
// for this module's own two loyalty-adjustment endpoints.

const (
	loyaltyAuditJWTSecret = "test-secret-at-least-32-bytes-long-for-hs256"
	loyaltyAuditIssuer    = "mesaposta-api-test"
	loyaltyAuditAudience  = "mesaposta-api-test"
	loyaltyAuditKID       = "test-kid"
	loyaltyAuditSubject   = "user-88"
)

func newLoyaltyAuthenticatedTestRouter(t *testing.T, uow *fakeUnitOfWork) chi.Router {
	t.Helper()
	verifier, err := token.NewVerifier(
		map[string][]byte{loyaltyAuditKID: []byte(loyaltyAuditJWTSecret)}, loyaltyAuditIssuer, loyaltyAuditAudience)
	if err != nil {
		t.Fatalf("token.NewVerifier: %v", err)
	}

	r := chi.NewRouter()
	r.Use(auth.Authenticate(verifier))
	crmhttp.Mount(r, uow)
	return r
}

func mustLoyaltyAuditAccessToken(t *testing.T, tenantID, subject string) string {
	t.Helper()
	issuer, err := token.NewIssuer(loyaltyAuditKID, []byte(loyaltyAuditJWTSecret), loyaltyAuditIssuer, loyaltyAuditAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	tok, err := issuer.Issue(subject, tenantID, token.RoleGestor, "session-"+subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

func TestCreditLoyaltyPointsHandler_RecordsAuthenticatedSubjectAsAuditActor(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	body, err := json.Marshal(map[string]any{"points": 50, "reason": "Pedido #1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/credit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mustLoyaltyAuditAccessToken(t, testTenantIDStr, loyaltyAuditSubject))

	// Act
	w := httptest.NewRecorder()
	newLoyaltyAuthenticatedTestRouter(t, uow).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	recorded := uow.snapshot().auditEntries
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	entry := recorded[0]
	if entry.ActorID != loyaltyAuditSubject {
		t.Errorf("entry.ActorID = %q, want %q (the JWT \"sub\" claim)", entry.ActorID, loyaltyAuditSubject)
	}
	if entry.Action != "loyalty_entry.credited" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "loyalty_entry.credited")
	}
}

func TestRedeemLoyaltyPointsHandler_RecordsAuthenticatedSubjectAsAuditActor(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	if err := customer.CreditPoints(mustFakeHTTPLoyaltyPoints(t, 100)); err != nil {
		t.Fatalf("test setup: CreditPoints: %v", err)
	}
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	body, err := json.Marshal(map[string]any{"points": 30, "reason": "Resgate de brinde"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/redeem", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mustLoyaltyAuditAccessToken(t, testTenantIDStr, loyaltyAuditSubject))

	// Act
	w := httptest.NewRecorder()
	newLoyaltyAuthenticatedTestRouter(t, uow).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	recorded := uow.snapshot().auditEntries
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	entry := recorded[0]
	if entry.ActorID != loyaltyAuditSubject {
		t.Errorf("entry.ActorID = %q, want %q (the JWT \"sub\" claim)", entry.ActorID, loyaltyAuditSubject)
	}
	if entry.Action != "loyalty_entry.redeemed" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "loyalty_entry.redeemed")
	}
}

// TestCreditLoyaltyPointsHandler_NoClaims_RecordsEmptyActorAndStillSucceeds
// pins down the "audit must never block the operation" rule.
func TestCreditLoyaltyPointsHandler_NoClaims_RecordsEmptyActorAndStillSucceeds(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	customer := mustHTTPCustomer(t, tenantID)
	state := newFakeState()
	state.customers[customer.ID()] = customer
	uow := newFakeUnitOfWork(state)

	body, err := json.Marshal(map[string]any{"points": 50, "reason": "Pedido #1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	ctx := tenant.With(context.Background(), tenantID)
	req := httptest.NewRequest(http.MethodPost, "/customers/"+customer.ID().String()+"/loyalty/credit", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	// Act -- newTestRouter mounts the module WITHOUT auth.Authenticate, so
	// the request context carries a tenant but no token.Claims.
	w := httptest.NewRecorder()
	newTestRouter(uow).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	recorded := uow.snapshot().auditEntries
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	if got := recorded[0].ActorID; got != "" {
		t.Errorf("entry.ActorID = %q, want %q (no claims means no actor, not a placeholder)", got, "")
	}
}
