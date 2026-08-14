package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	orderinghttp "github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/infrastructure/http"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
)

// This file covers ONE thing this package's other handler tests cannot:
// that the "who" recorded in the audit trail is really the authenticated
// caller, all the way from the Authorization header to the audit.Entry --
// mirroring
// internal/modules/operations/infrastructure/http/receive_purchase_order_audit_test.go
// for this module's own order-status-transition endpoints.

const (
	orderAuditJWTSecret = "test-secret-at-least-32-bytes-long-for-hs256"
	orderAuditIssuer    = "mesaposta-api-test"
	orderAuditAudience  = "mesaposta-api-test"
	orderAuditKID       = "test-kid"
	orderAuditSubject   = "user-99"
)

func newOrderAuthenticatedTestRouter(t *testing.T, uow *fakeUnitOfWork) chi.Router {
	t.Helper()
	verifier, err := token.NewVerifier(
		map[string][]byte{orderAuditKID: []byte(orderAuditJWTSecret)}, orderAuditIssuer, orderAuditAudience)
	if err != nil {
		t.Fatalf("token.NewVerifier: %v", err)
	}

	r := chi.NewRouter()
	r.Use(auth.Authenticate(verifier))
	orderinghttp.MountTables(r, uow)
	orderinghttp.MountOrders(r, uow)
	return r
}

func mustOrderAuditAccessToken(t *testing.T, tenantID, subject string) string {
	t.Helper()
	issuer, err := token.NewIssuer(orderAuditKID, []byte(orderAuditJWTSecret), orderAuditIssuer, orderAuditAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	tok, err := issuer.Issue(subject, tenantID, token.RoleGestor, "session-"+subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

func TestConfirmOrderHandler_RecordsAuthenticatedSubjectAsAuditActor(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/confirmar", nil)
	req.Header.Set("Authorization", "Bearer "+mustOrderAuditAccessToken(t, testTenantIDStr, orderAuditSubject))

	// Act
	w := httptest.NewRecorder()
	newOrderAuthenticatedTestRouter(t, uow).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	recorded := uow.snapshot().auditEntries
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	entry := recorded[0]
	if entry.ActorID != orderAuditSubject {
		t.Errorf("entry.ActorID = %q, want %q (the JWT \"sub\" claim)", entry.ActorID, orderAuditSubject)
	}
	if entry.Action != "order.status_changed" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "order.status_changed")
	}
	if entry.EntityID != order.ID().String() {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, order.ID().String())
	}
}

// TestConfirmOrderHandler_NoClaims_RecordsEmptyActorAndStillSucceeds pins
// down the "audit must never block the operation" rule.
func TestConfirmOrderHandler_NoClaims_RecordsEmptyActorAndStillSucceeds(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	table := mustHTTPTable(t, tenantID)
	order := mustHTTPOrder(t, tenantID, table.ID())
	state := newFakeState()
	state.orders[order.ID()] = order
	uow := newFakeUnitOfWork(state)

	req := httptest.NewRequest(http.MethodPost, "/pedidos/"+order.ID().String()+"/confirmar", nil)

	// Act -- newTestRouter mounts the module WITHOUT auth.Authenticate, so
	// the request context carries a tenant (via doRequest) but no
	// token.Claims.
	w := doRequest(newTestRouter(uow), req, true, tenantID)

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
