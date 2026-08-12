package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const stockItemTestTenantID = "11111111-2222-3333-4444-555555555555"

// mustQuantity and mustMoney are shared test helpers already declared in
// quantity_test.go and money_test.go (same package domain_test) -- reused
// here rather than redeclared.

// mustTenantID is a test helper that fails the test immediately if
// tenant.Parse returns an error.
func mustTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(stockItemTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q) returned unexpected error: %v", stockItemTestTenantID, err)
	}
	return id
}

// newTestStockItem builds a valid StockItem with 10.000 units of stock,
// ready for Withdraw/Receive tests.
func newTestStockItem(t *testing.T) domain.StockItem {
	t.Helper()
	item, err := domain.NewStockItem(
		domain.NewID(),
		mustTenantID(t),
		"Farinha de trigo",
		domain.StockUnitKg,
		mustQuantity(t, 10000),
		mustQuantity(t, 1000),
		mustMoney(t, 500),
		nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	return item
}

func TestNewStockItem_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	qty := mustQuantity(t, 5000)
	minQty := mustQuantity(t, 1000)
	cost := mustMoney(t, 250)
	supplierID := domain.NewID()

	// Act
	item, err := domain.NewStockItem(id, tenantID, "Acucar", domain.StockUnitKg, qty, minQty, cost, &supplierID)

	// Assert
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	if item.ID() != id {
		t.Errorf("item.ID() = %v, want %v", item.ID(), id)
	}
	if item.TenantID() != tenantID {
		t.Errorf("item.TenantID() = %v, want %v", item.TenantID(), tenantID)
	}
	if item.Name() != "Acucar" {
		t.Errorf("item.Name() = %q, want %q", item.Name(), "Acucar")
	}
	if item.Unit() != domain.StockUnitKg {
		t.Errorf("item.Unit() = %v, want %v", item.Unit(), domain.StockUnitKg)
	}
	if item.Quantity() != qty {
		t.Errorf("item.Quantity() = %v, want %v", item.Quantity(), qty)
	}
	if item.MinQuantity() != minQty {
		t.Errorf("item.MinQuantity() = %v, want %v", item.MinQuantity(), minQty)
	}
	if item.CostPerUnit() != cost {
		t.Errorf("item.CostPerUnit() = %v, want %v", item.CostPerUnit(), cost)
	}
	if item.SupplierID() == nil || *item.SupplierID() != supplierID {
		t.Errorf("item.SupplierID() = %v, want %v", item.SupplierID(), supplierID)
	}
	if item.Version() != 1 {
		t.Errorf("item.Version() = %d, want 1", item.Version())
	}
	if item.UpdatedAt().IsZero() {
		t.Errorf("item.UpdatedAt() is zero, want a populated timestamp")
	}
}

func TestNewStockItem_NilSupplierID_Succeeds(t *testing.T) {
	// Arrange & Act
	item, err := domain.NewStockItem(
		domain.NewID(), mustTenantID(t), "Sal", domain.StockUnitKg,
		mustQuantity(t, 1000), mustQuantity(t, 100), mustMoney(t, 100), nil,
	)

	// Assert
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	if item.SupplierID() != nil {
		t.Errorf("item.SupplierID() = %v, want nil", item.SupplierID())
	}
}

func TestNewStockItem_EmptyName_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewStockItem(
		domain.NewID(), mustTenantID(t), "", domain.StockUnitKg,
		mustQuantity(t, 1000), mustQuantity(t, 100), mustMoney(t, 100), nil,
	)

	// Assert
	if err == nil {
		t.Fatal("NewStockItem with empty name returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewStockItem_InvalidUnit_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewStockItem(
		domain.NewID(), mustTenantID(t), "Item", domain.StockUnit("invalid"),
		mustQuantity(t, 1000), mustQuantity(t, 100), mustMoney(t, 100), nil,
	)

	// Assert
	if err == nil {
		t.Fatal("NewStockItem with invalid unit returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

// TestStockItem_Withdraw_MoreThanBalance_ReturnsConflictAndLeavesStateIntact
// is the single most important test in this file (acceptance criterion 1):
// withdrawing more stock than is on hand must be rejected, and rejected
// without mutating the aggregate at all.
func TestStockItem_Withdraw_MoreThanBalance_ReturnsConflictAndLeavesStateIntact(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.Quantity()
	beforeVersion := item.Version()
	beforeUpdatedAt := item.UpdatedAt()
	tooMuch := mustQuantity(t, 20000) // more than the 10.000 on hand

	// Act
	err := item.Withdraw(tooMuch)

	// Assert: the error.
	if err == nil {
		t.Fatal("Withdraw(more than balance) returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	// Assert: the aggregate state is untouched.
	if item.Quantity() != before {
		t.Errorf("item.Quantity() = %v after failed Withdraw, want unchanged %v", item.Quantity(), before)
	}
	if item.Version() != beforeVersion {
		t.Errorf("item.Version() = %d after failed Withdraw, want unchanged %d", item.Version(), beforeVersion)
	}
	if item.UpdatedAt() != beforeUpdatedAt {
		t.Errorf("item.UpdatedAt() = %v after failed Withdraw, want unchanged %v", item.UpdatedAt(), beforeUpdatedAt)
	}
}

func TestStockItem_Withdraw_ExactBalance_SucceedsAndReachesZero(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	all := item.Quantity()
	beforeVersion := item.Version()

	// Act
	err := item.Withdraw(all)

	// Assert
	if err != nil {
		t.Fatalf("Withdraw(entire balance) returned unexpected error: %v", err)
	}
	if item.Quantity().Thousandths() != 0 {
		t.Errorf("item.Quantity() = %v after withdrawing entire balance, want zero", item.Quantity())
	}
	if item.Version() != beforeVersion+1 {
		t.Errorf("item.Version() = %d, want %d", item.Version(), beforeVersion+1)
	}
}

func TestStockItem_Withdraw_LessThanBalance_SucceedsAndDecrementsByExactAmount(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.Quantity()
	withdrawal := mustQuantity(t, 3000)

	// Act
	err := item.Withdraw(withdrawal)

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned unexpected error: %v", err)
	}
	want, subErr := before.Sub(withdrawal)
	if subErr != nil {
		t.Fatalf("test setup: before.Sub(withdrawal) returned unexpected error: %v", subErr)
	}
	if item.Quantity() != want {
		t.Errorf("item.Quantity() = %v, want %v", item.Quantity(), want)
	}
}

func TestStockItem_Withdraw_Zero_ReturnsInvalidError(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.Quantity()
	beforeVersion := item.Version()
	zero := mustQuantity(t, 0)

	// Act
	err := item.Withdraw(zero)

	// Assert
	if err == nil {
		t.Fatal("Withdraw(0) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if item.Quantity() != before {
		t.Errorf("item.Quantity() = %v after failed Withdraw(0), want unchanged %v", item.Quantity(), before)
	}
	if item.Version() != beforeVersion {
		t.Errorf("item.Version() = %d after failed Withdraw(0), want unchanged %d", item.Version(), beforeVersion)
	}
}

func TestStockItem_Withdraw_NegativeQuantityCannotBeConstructed(t *testing.T) {
	// Arrange & Act: Quantity's own constructor is the guard against a
	// negative withdrawal amount ever reaching Withdraw in the first place.
	_, err := domain.NewQuantity(-1)

	// Assert
	if err == nil {
		t.Fatal("NewQuantity(-1) returned nil error, want an error (Quantity must never be negative)")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestStockItem_Receive_AddsToQuantityAndIncrementsVersion(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.Quantity()
	beforeVersion := item.Version()
	addition := mustQuantity(t, 2500)

	// Act
	err := item.Receive(addition)

	// Assert
	if err != nil {
		t.Fatalf("Receive returned unexpected error: %v", err)
	}
	want, addErr := before.Add(addition)
	if addErr != nil {
		t.Fatalf("test setup: before.Add(addition) returned unexpected error: %v", addErr)
	}
	if item.Quantity() != want {
		t.Errorf("item.Quantity() = %v, want %v", item.Quantity(), want)
	}
	if item.Version() != beforeVersion+1 {
		t.Errorf("item.Version() = %d, want %d", item.Version(), beforeVersion+1)
	}
}

func TestStockItem_Receive_Zero_ReturnsInvalidError(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.Quantity()
	beforeVersion := item.Version()
	zero := mustQuantity(t, 0)

	// Act
	err := item.Receive(zero)

	// Assert
	if err == nil {
		t.Fatal("Receive(0) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if item.Quantity() != before {
		t.Errorf("item.Quantity() = %v after failed Receive(0), want unchanged %v", item.Quantity(), before)
	}
	if item.Version() != beforeVersion {
		t.Errorf("item.Version() = %d after failed Receive(0), want unchanged %d", item.Version(), beforeVersion)
	}
}

func TestStockItem_Receive_OverflowPropagatesError(t *testing.T) {
	// Arrange: build a stock item holding math.MaxInt64 thousandths, then
	// try to receive one more unit so Quantity.Add overflows.
	maxQty := mustQuantity(t, 9223372036854775807) // math.MaxInt64
	item, err := domain.NewStockItem(
		domain.NewID(), mustTenantID(t), "Item cheio", domain.StockUnitKg,
		maxQty, mustQuantity(t, 0), mustMoney(t, 100), nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem returned unexpected error: %v", err)
	}
	beforeVersion := item.Version()

	// Act
	err = item.Receive(mustQuantity(t, 1))

	// Assert
	if err == nil {
		t.Fatal("Receive(overflow) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if item.Quantity() != maxQty {
		t.Errorf("item.Quantity() = %v after failed Receive, want unchanged %v", item.Quantity(), maxQty)
	}
	if item.Version() != beforeVersion {
		t.Errorf("item.Version() = %d after failed Receive, want unchanged %d", item.Version(), beforeVersion)
	}
}

func TestStockItem_UpdatedAt_ChangesOnSuccessfulMutation(t *testing.T) {
	// Arrange
	item := newTestStockItem(t)
	before := item.UpdatedAt()
	time.Sleep(1 * time.Millisecond)

	// Act
	if err := item.Receive(mustQuantity(t, 1)); err != nil {
		t.Fatalf("Receive returned unexpected error: %v", err)
	}

	// Assert
	if !item.UpdatedAt().After(before) {
		t.Errorf("item.UpdatedAt() = %v, want after %v", item.UpdatedAt(), before)
	}
}
