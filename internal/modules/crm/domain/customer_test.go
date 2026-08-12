package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// newTestCustomer builds a valid Customer with a zero loyalty point
// balance, ready for CreditPoints/RedeemPoints tests.
func newTestCustomer(t *testing.T) domain.Customer {
	t.Helper()
	email := "maria@example.com"
	customer, err := domain.NewCustomer(domain.NewID(), mustCRMTenantID(t), "Maria", &email, nil)
	if err != nil {
		t.Fatalf("NewCustomer returned unexpected error: %v", err)
	}
	return customer
}

// newTestCustomerWithBalance builds a valid Customer and credits it with
// balance loyalty points, ready for RedeemPoints tests.
func newTestCustomerWithBalance(t *testing.T, balance int64) domain.Customer {
	t.Helper()
	customer := newTestCustomer(t)
	if err := customer.CreditPoints(mustLoyaltyPoints(t, balance)); err != nil {
		t.Fatalf("test setup: CreditPoints(%d) returned unexpected error: %v", balance, err)
	}
	return customer
}

func TestNewCustomer_ValidInput_Succeeds(t *testing.T) {
	id := domain.NewID()
	tenantID := mustCRMTenantID(t)
	email := "joao@example.com"
	phone := "+5511999999999"

	customer, err := domain.NewCustomer(id, tenantID, "Joao", &email, &phone)
	if err != nil {
		t.Fatalf("NewCustomer returned unexpected error: %v", err)
	}
	if customer.ID() != id {
		t.Errorf("customer.ID() = %v, want %v", customer.ID(), id)
	}
	if customer.TenantID() != tenantID {
		t.Errorf("customer.TenantID() = %v, want %v", customer.TenantID(), tenantID)
	}
	if customer.Name() != "Joao" {
		t.Errorf("customer.Name() = %q, want %q", customer.Name(), "Joao")
	}
	if customer.Email() == nil || *customer.Email() != email {
		t.Errorf("customer.Email() = %v, want %v", customer.Email(), email)
	}
	if customer.Phone() == nil || *customer.Phone() != phone {
		t.Errorf("customer.Phone() = %v, want %v", customer.Phone(), phone)
	}
	if customer.LoyaltyPoints().Points() != 0 {
		t.Errorf("customer.LoyaltyPoints().Points() = %d, want 0", customer.LoyaltyPoints().Points())
	}
	if customer.Version() != 1 {
		t.Errorf("customer.Version() = %d, want 1", customer.Version())
	}
	if customer.CreatedAt().IsZero() {
		t.Error("customer.CreatedAt() is zero, want a populated timestamp")
	}
	if customer.UpdatedAt().IsZero() {
		t.Error("customer.UpdatedAt() is zero, want a populated timestamp")
	}
}

func TestNewCustomer_NilEmailAndPhone_Succeeds(t *testing.T) {
	customer, err := domain.NewCustomer(domain.NewID(), mustCRMTenantID(t), "Sem Contato", nil, nil)
	if err != nil {
		t.Fatalf("NewCustomer returned unexpected error: %v", err)
	}
	if customer.Email() != nil {
		t.Errorf("customer.Email() = %v, want nil", customer.Email())
	}
	if customer.Phone() != nil {
		t.Errorf("customer.Phone() = %v, want nil", customer.Phone())
	}
}

func TestNewCustomer_EmptyName_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewCustomer(domain.NewID(), mustCRMTenantID(t), "", nil, nil)
	if err == nil {
		t.Fatal("NewCustomer(empty name) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewCustomer_BlankEmail_ReturnsInvalidError(t *testing.T) {
	blank := "   "
	_, err := domain.NewCustomer(domain.NewID(), mustCRMTenantID(t), "Nome", &blank, nil)
	if err == nil {
		t.Fatal("NewCustomer(blank email) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewCustomer_BlankPhone_ReturnsInvalidError(t *testing.T) {
	blank := ""
	_, err := domain.NewCustomer(domain.NewID(), mustCRMTenantID(t), "Nome", nil, &blank)
	if err == nil {
		t.Fatal("NewCustomer(blank phone) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestCustomer_CreditPoints_AddsToBalanceAndIncrementsVersion(t *testing.T) {
	customer := newTestCustomer(t)
	beforeVersion := customer.Version()

	if err := customer.CreditPoints(mustLoyaltyPoints(t, 100)); err != nil {
		t.Fatalf("CreditPoints returned unexpected error: %v", err)
	}
	if customer.LoyaltyPoints().Points() != 100 {
		t.Errorf("customer.LoyaltyPoints().Points() = %d, want 100", customer.LoyaltyPoints().Points())
	}
	if customer.Version() != beforeVersion+1 {
		t.Errorf("customer.Version() = %d, want %d", customer.Version(), beforeVersion+1)
	}
}

func TestCustomer_CreditPoints_Zero_ReturnsInvalidErrorAndLeavesStateIntact(t *testing.T) {
	customer := newTestCustomer(t)
	beforeVersion := customer.Version()

	err := customer.CreditPoints(mustLoyaltyPoints(t, 0))
	if err == nil {
		t.Fatal("CreditPoints(0) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if customer.LoyaltyPoints().Points() != 0 {
		t.Errorf("customer.LoyaltyPoints().Points() = %d after failed CreditPoints(0), want unchanged 0", customer.LoyaltyPoints().Points())
	}
	if customer.Version() != beforeVersion {
		t.Errorf("customer.Version() = %d after failed CreditPoints(0), want unchanged %d", customer.Version(), beforeVersion)
	}
}

// TestCustomer_RedeemPoints_MoreThanBalance_ReturnsConflictAndLeavesStateIntact
// is the single most important test in this package (this module's
// headline acceptance criterion): redeeming more loyalty points than the
// customer's current balance must be rejected with a Conflict error, and
// rejected WITHOUT mutating the aggregate at all -- mirroring
// internal/modules/operations/domain's own
// TestStockItem_Withdraw_MoreThanBalance_ReturnsConflictAndLeavesStateIntact.
func TestCustomer_RedeemPoints_MoreThanBalance_ReturnsConflictAndLeavesStateIntact(t *testing.T) {
	customer := newTestCustomerWithBalance(t, 100)
	beforeBalance := customer.LoyaltyPoints()
	beforeVersion := customer.Version()
	beforeUpdatedAt := customer.UpdatedAt()
	tooMuch := mustLoyaltyPoints(t, 150) // more than the 100 on the balance

	err := customer.RedeemPoints(tooMuch)

	// Assert: the error.
	if err == nil {
		t.Fatal("RedeemPoints(more than balance) returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	// Assert: the aggregate state is untouched.
	if customer.LoyaltyPoints() != beforeBalance {
		t.Errorf("customer.LoyaltyPoints() = %v after failed RedeemPoints, want unchanged %v", customer.LoyaltyPoints(), beforeBalance)
	}
	if customer.Version() != beforeVersion {
		t.Errorf("customer.Version() = %d after failed RedeemPoints, want unchanged %d", customer.Version(), beforeVersion)
	}
	if customer.UpdatedAt() != beforeUpdatedAt {
		t.Errorf("customer.UpdatedAt() = %v after failed RedeemPoints, want unchanged %v", customer.UpdatedAt(), beforeUpdatedAt)
	}
}

func TestCustomer_RedeemPoints_ExactBalance_SucceedsAndReachesZero(t *testing.T) {
	customer := newTestCustomerWithBalance(t, 100)
	all := customer.LoyaltyPoints()
	beforeVersion := customer.Version()

	if err := customer.RedeemPoints(all); err != nil {
		t.Fatalf("RedeemPoints(entire balance) returned unexpected error: %v", err)
	}
	if customer.LoyaltyPoints().Points() != 0 {
		t.Errorf("customer.LoyaltyPoints().Points() = %d after redeeming entire balance, want 0", customer.LoyaltyPoints().Points())
	}
	if customer.Version() != beforeVersion+1 {
		t.Errorf("customer.Version() = %d, want %d", customer.Version(), beforeVersion+1)
	}
}

func TestCustomer_RedeemPoints_LessThanBalance_SucceedsAndDecrementsByExactAmount(t *testing.T) {
	customer := newTestCustomerWithBalance(t, 100)

	if err := customer.RedeemPoints(mustLoyaltyPoints(t, 30)); err != nil {
		t.Fatalf("RedeemPoints returned unexpected error: %v", err)
	}
	if customer.LoyaltyPoints().Points() != 70 {
		t.Errorf("customer.LoyaltyPoints().Points() = %d, want 70", customer.LoyaltyPoints().Points())
	}
}

func TestCustomer_RedeemPoints_Zero_ReturnsInvalidErrorAndLeavesStateIntact(t *testing.T) {
	customer := newTestCustomerWithBalance(t, 100)
	beforeBalance := customer.LoyaltyPoints()
	beforeVersion := customer.Version()

	err := customer.RedeemPoints(mustLoyaltyPoints(t, 0))
	if err == nil {
		t.Fatal("RedeemPoints(0) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if customer.LoyaltyPoints() != beforeBalance {
		t.Errorf("customer.LoyaltyPoints() = %v after failed RedeemPoints(0), want unchanged %v", customer.LoyaltyPoints(), beforeBalance)
	}
	if customer.Version() != beforeVersion {
		t.Errorf("customer.Version() = %d after failed RedeemPoints(0), want unchanged %d", customer.Version(), beforeVersion)
	}
}

func TestCustomer_RedeemPoints_FromZeroBalance_ReturnsConflict(t *testing.T) {
	customer := newTestCustomer(t) // zero balance
	err := customer.RedeemPoints(mustLoyaltyPoints(t, 1))
	if err == nil {
		t.Fatal("RedeemPoints(from zero balance) returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
}

func TestCustomer_UpdatedAt_ChangesOnSuccessfulMutation(t *testing.T) {
	customer := newTestCustomer(t)
	before := customer.UpdatedAt()
	time.Sleep(1 * time.Millisecond)

	if err := customer.CreditPoints(mustLoyaltyPoints(t, 1)); err != nil {
		t.Fatalf("CreditPoints returned unexpected error: %v", err)
	}
	if !customer.UpdatedAt().After(before) {
		t.Errorf("customer.UpdatedAt() = %v, want after %v", customer.UpdatedAt(), before)
	}
}
