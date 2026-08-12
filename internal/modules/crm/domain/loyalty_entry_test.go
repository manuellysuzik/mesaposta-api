package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const loyaltyEntryTestTenantID = "11111111-2222-3333-4444-555555555555"

func mustCRMTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(loyaltyEntryTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q) returned unexpected error: %v", loyaltyEntryTestTenantID, err)
	}
	return id
}

func TestNewLoyaltyEntry_ValidInput_Succeeds(t *testing.T) {
	id := domain.NewID()
	tenantID := mustCRMTenantID(t)
	customerID := domain.NewID()
	points := mustLoyaltyPoints(t, 50)

	entry, err := domain.NewLoyaltyEntry(id, tenantID, customerID, domain.LoyaltyEntryTypeCredito, points, "Compra #123")
	if err != nil {
		t.Fatalf("NewLoyaltyEntry returned unexpected error: %v", err)
	}
	if entry.ID() != id {
		t.Errorf("entry.ID() = %v, want %v", entry.ID(), id)
	}
	if entry.TenantID() != tenantID {
		t.Errorf("entry.TenantID() = %v, want %v", entry.TenantID(), tenantID)
	}
	if entry.CustomerID() != customerID {
		t.Errorf("entry.CustomerID() = %v, want %v", entry.CustomerID(), customerID)
	}
	if entry.EntryType() != domain.LoyaltyEntryTypeCredito {
		t.Errorf("entry.EntryType() = %v, want %v", entry.EntryType(), domain.LoyaltyEntryTypeCredito)
	}
	if entry.Points() != points {
		t.Errorf("entry.Points() = %v, want %v", entry.Points(), points)
	}
	if entry.Reason() != "Compra #123" {
		t.Errorf("entry.Reason() = %q, want %q", entry.Reason(), "Compra #123")
	}
	if entry.CreatedAt().IsZero() {
		t.Error("entry.CreatedAt() is zero, want a populated timestamp")
	}
}

func TestNewLoyaltyEntry_ZeroCustomerID_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewLoyaltyEntry(domain.NewID(), mustCRMTenantID(t), domain.ID{}, domain.LoyaltyEntryTypeCredito, mustLoyaltyPoints(t, 10), "reason")
	if err == nil {
		t.Fatal("NewLoyaltyEntry(zero customer id) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewLoyaltyEntry_InvalidType_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewLoyaltyEntry(domain.NewID(), mustCRMTenantID(t), domain.NewID(), domain.LoyaltyEntryType("bogus"), mustLoyaltyPoints(t, 10), "reason")
	if err == nil {
		t.Fatal("NewLoyaltyEntry(invalid type) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewLoyaltyEntry_ZeroPoints_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewLoyaltyEntry(domain.NewID(), mustCRMTenantID(t), domain.NewID(), domain.LoyaltyEntryTypeCredito, mustLoyaltyPoints(t, 0), "reason")
	if err == nil {
		t.Fatal("NewLoyaltyEntry(zero points) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewLoyaltyEntry_EmptyReason_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewLoyaltyEntry(domain.NewID(), mustCRMTenantID(t), domain.NewID(), domain.LoyaltyEntryTypeCredito, mustLoyaltyPoints(t, 10), "")
	if err == nil {
		t.Fatal("NewLoyaltyEntry(empty reason) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}
