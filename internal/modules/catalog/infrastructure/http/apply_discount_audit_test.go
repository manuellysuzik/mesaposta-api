package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	cataloghttp "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/http"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file covers ONE thing this package's other handler tests cannot:
// that the "who" recorded in the audit trail is really the authenticated
// caller, all the way from the Authorization header to the audit.Entry --
// mirroring
// internal/modules/operations/infrastructure/http/receive_purchase_order_audit_test.go
// exactly, one module over.

const (
	discountAuditJWTSecret = "test-secret-at-least-32-bytes-long-for-hs256"
	discountAuditIssuer    = "mesaposta-api-test"
	discountAuditAudience  = "mesaposta-api-test"
	discountAuditKID       = "test-kid"
	discountAuditSubject   = "user-77"
)

// newAuthenticatedTestRouter mounts the catalog module behind the real
// auth.Authenticate middleware, mirroring how internal/platform/app wires
// it in production.
func newAuthenticatedTestRouter(t *testing.T, uow *fakeUnitOfWork) chi.Router {
	t.Helper()
	verifier, err := token.NewVerifier(
		map[string][]byte{discountAuditKID: []byte(discountAuditJWTSecret)}, discountAuditIssuer, discountAuditAudience)
	if err != nil {
		t.Fatalf("token.NewVerifier: %v", err)
	}

	r := chi.NewRouter()
	r.Use(auth.Authenticate(verifier))
	cataloghttp.Mount(r, uow)
	return r
}

func mustDiscountAuditAccessToken(t *testing.T, tenantID, subject string) string {
	t.Helper()
	issuer, err := token.NewIssuer(discountAuditKID, []byte(discountAuditJWTSecret), discountAuditIssuer, discountAuditAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	tok, err := issuer.Issue(subject, tenantID, token.RoleManager, "session-"+subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

func TestApplyDiscountHandler_RecordsAuthenticatedSubjectAsAuditActor(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)

	body, err := json.Marshal(map[string]any{"discount_percent_hundredths": 2500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/"+product.ID().String()+"/discount", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mustDiscountAuditAccessToken(t, testTenantIDStr, discountAuditSubject))

	// Act
	w := httptest.NewRecorder()
	newAuthenticatedTestRouter(t, uow).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	recorded := uow.snapshot().auditEntries
	if len(recorded) != 1 {
		t.Fatalf("len(recorded audit entries) = %d, want exactly 1: %+v", len(recorded), recorded)
	}
	entry := recorded[0]
	if entry.ActorID != discountAuditSubject {
		t.Errorf("entry.ActorID = %q, want %q (the JWT \"sub\" claim)", entry.ActorID, discountAuditSubject)
	}
	if entry.Action != "product.discount_applied" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "product.discount_applied")
	}
	if entry.EntityID != product.ID().String() {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, product.ID().String())
	}
}

// TestApplyDiscountHandler_NoClaims_RecordsEmptyActorAndStillSucceeds pins
// down the "audit must never block the operation" rule: a request carrying
// a tenant but no claims still applies the discount -- it just records no
// actor, rather than failing or inventing one.
func TestApplyDiscountHandler_NoClaims_RecordsEmptyActorAndStillSucceeds(t *testing.T) {
	// Arrange
	tenantID := mustHTTPTenantID(t)
	product := mustHTTPProduct(t, tenantID)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)

	body, err := json.Marshal(map[string]any{"discount_percent_hundredths": 2500})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	ctx := tenant.With(context.Background(), tenantID)
	req := httptest.NewRequest(http.MethodPost, "/products/"+product.ID().String()+"/discount", bytes.NewReader(body)).WithContext(ctx)
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
