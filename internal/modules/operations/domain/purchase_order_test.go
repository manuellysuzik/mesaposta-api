package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// mustPurchaseOrderItem is a test helper that fails the test immediately if
// domain.NewPurchaseOrderItem returns an error.
func mustPurchaseOrderItem(t *testing.T, stockItemID domain.ID, quantity domain.Quantity, costPerUnit domain.Money) domain.PurchaseOrderItem {
	t.Helper()
	item, err := domain.NewPurchaseOrderItem(stockItemID, quantity, costPerUnit)
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem returned unexpected error: %v", err)
	}
	return item
}

// newTestPurchaseOrder builds a valid, pending PurchaseOrder with two
// items, ready for use in Total/MarkAsReceived/Cancel tests.
func newTestPurchaseOrder(t *testing.T) domain.PurchaseOrder {
	t.Helper()
	items := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, domain.NewID(), mustQuantity(t, 2000), mustMoney(t, 350)),
		mustPurchaseOrderItem(t, domain.NewID(), mustQuantity(t, 1500), mustMoney(t, 900)),
	}
	order, err := domain.NewPurchaseOrder(domain.NewID(), mustTenantID(t), domain.NewID(), items)
	if err != nil {
		t.Fatalf("NewPurchaseOrder returned unexpected error: %v", err)
	}
	return order
}

func TestNewPurchaseOrderItem_ZeroStockItemID_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewPurchaseOrderItem(domain.ID{}, mustQuantity(t, 1000), mustMoney(t, 500))

	// Assert
	if err == nil {
		t.Fatal("NewPurchaseOrderItem with zero stock item ID returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewPurchaseOrderItem_ZeroQuantity_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewPurchaseOrderItem(domain.NewID(), mustQuantity(t, 0), mustMoney(t, 500))

	// Assert
	if err == nil {
		t.Fatal("NewPurchaseOrderItem with zero quantity returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewPurchaseOrder_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	supplierID := domain.NewID()
	items := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, domain.NewID(), mustQuantity(t, 1000), mustMoney(t, 500)),
	}

	// Act
	order, err := domain.NewPurchaseOrder(id, tenantID, supplierID, items)

	// Assert
	if err != nil {
		t.Fatalf("NewPurchaseOrder returned unexpected error: %v", err)
	}
	if order.ID() != id {
		t.Errorf("order.ID() = %v, want %v", order.ID(), id)
	}
	if order.TenantID() != tenantID {
		t.Errorf("order.TenantID() = %v, want %v", order.TenantID(), tenantID)
	}
	if order.SupplierID() != supplierID {
		t.Errorf("order.SupplierID() = %v, want %v", order.SupplierID(), supplierID)
	}
	if order.Status() != domain.PurchaseOrderStatusPendente {
		t.Errorf("order.Status() = %v, want %v", order.Status(), domain.PurchaseOrderStatusPendente)
	}
	if len(order.Items()) != 1 {
		t.Fatalf("len(order.Items()) = %d, want 1", len(order.Items()))
	}
	if order.Version() != 1 {
		t.Errorf("order.Version() = %d, want 1", order.Version())
	}
	if order.CreatedAt().IsZero() {
		t.Error("order.CreatedAt() is zero, want a populated timestamp")
	}
	if order.ReceivedAt() != nil {
		t.Errorf("order.ReceivedAt() = %v, want nil", order.ReceivedAt())
	}
}

func TestNewPurchaseOrder_NoItems_ReturnsInvalidError(t *testing.T) {
	// Arrange & Act
	_, err := domain.NewPurchaseOrder(domain.NewID(), mustTenantID(t), domain.NewID(), nil)

	// Assert
	if err == nil {
		t.Fatal("NewPurchaseOrder with no items returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewPurchaseOrder_DuplicateStockItemID_ReturnsInvalidError(t *testing.T) {
	// Arrange
	dupID := domain.NewID()
	items := []domain.PurchaseOrderItem{
		mustPurchaseOrderItem(t, dupID, mustQuantity(t, 1000), mustMoney(t, 500)),
		mustPurchaseOrderItem(t, dupID, mustQuantity(t, 500), mustMoney(t, 500)),
	}

	// Act
	_, err := domain.NewPurchaseOrder(domain.NewID(), mustTenantID(t), domain.NewID(), items)

	// Assert
	if err == nil {
		t.Fatal("NewPurchaseOrder with duplicate stock item ID returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestPurchaseOrder_Total_SumsItemsCorrectly(t *testing.T) {
	// Arrange: item 1 = 2.000 units @ R$3.50/unit = R$7.00 = 700 cents;
	// item 2 = 1.500 units @ R$9.00/unit = R$13.50 = 1350 cents.
	// Total = 700 + 1350 = 2050 cents.
	order := newTestPurchaseOrder(t)

	// Act
	total, err := order.Total()

	// Assert
	if err != nil {
		t.Fatalf("Total() returned unexpected error: %v", err)
	}
	if total.Cents() != 2050 {
		t.Errorf("Total().Cents() = %d, want 2050", total.Cents())
	}
}

func TestPurchaseOrder_MarkAsReceived_FirstCall_Succeeds(t *testing.T) {
	// Arrange
	order := newTestPurchaseOrder(t)
	now := time.Now().UTC()

	// Act
	err := order.MarkAsReceived(now)

	// Assert
	if err != nil {
		t.Fatalf("MarkAsReceived returned unexpected error: %v", err)
	}
	if order.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("order.Status() = %v, want %v", order.Status(), domain.PurchaseOrderStatusRecebido)
	}
	if order.ReceivedAt() == nil || !order.ReceivedAt().Equal(now) {
		t.Errorf("order.ReceivedAt() = %v, want %v", order.ReceivedAt(), now)
	}
	if order.Version() != 2 {
		t.Errorf("order.Version() = %d, want 2", order.Version())
	}
}

// TestPurchaseOrder_MarkAsReceived_SecondCall_ReturnsConflictAndLeavesStateIntact
// is the domain-level half of acceptance criterion 2: marking an
// already-received purchase order as received again must be rejected, and
// rejected without mutating the aggregate any further.
func TestPurchaseOrder_MarkAsReceived_SecondCall_ReturnsConflictAndLeavesStateIntact(t *testing.T) {
	// Arrange
	order := newTestPurchaseOrder(t)
	firstNow := time.Now().UTC()
	if err := order.MarkAsReceived(firstNow); err != nil {
		t.Fatalf("first MarkAsReceived returned unexpected error: %v", err)
	}
	statusAfterFirst := order.Status()
	versionAfterFirst := order.Version()
	receivedAtAfterFirst := order.ReceivedAt()

	// Act: call it again, with a different timestamp, to make sure a
	// second call cannot silently overwrite ReceivedAt either.
	secondNow := firstNow.Add(1 * time.Hour)
	err := order.MarkAsReceived(secondNow)

	// Assert: the error.
	if err == nil {
		t.Fatal("second MarkAsReceived returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	// Assert: the aggregate is left exactly as it was after the first call.
	if order.Status() != statusAfterFirst {
		t.Errorf("order.Status() = %v after second MarkAsReceived, want unchanged %v", order.Status(), statusAfterFirst)
	}
	if order.Version() != versionAfterFirst {
		t.Errorf("order.Version() = %d after second MarkAsReceived, want unchanged %d", order.Version(), versionAfterFirst)
	}
	if order.ReceivedAt() == nil || !order.ReceivedAt().Equal(*receivedAtAfterFirst) {
		t.Errorf("order.ReceivedAt() = %v after second MarkAsReceived, want unchanged %v", order.ReceivedAt(), receivedAtAfterFirst)
	}
}

func TestPurchaseOrder_MarkAsReceived_OnCancelledOrder_ReturnsConflictAndLeavesStateIntact(t *testing.T) {
	// Arrange: reach the cancelled state via Cancel, the minimal exposed
	// way to build this state for testing (see purchase_order.go's Cancel
	// doc comment).
	order := newTestPurchaseOrder(t)
	if err := order.Cancel(); err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	versionAfterCancel := order.Version()

	// Act
	err := order.MarkAsReceived(time.Now().UTC())

	// Assert
	if err == nil {
		t.Fatal("MarkAsReceived on a cancelled order returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
	if order.Status() != domain.PurchaseOrderStatusCancelado {
		t.Errorf("order.Status() = %v after failed MarkAsReceived, want unchanged %v", order.Status(), domain.PurchaseOrderStatusCancelado)
	}
	if order.Version() != versionAfterCancel {
		t.Errorf("order.Version() = %d after failed MarkAsReceived, want unchanged %d", order.Version(), versionAfterCancel)
	}
	if order.ReceivedAt() != nil {
		t.Errorf("order.ReceivedAt() = %v after failed MarkAsReceived, want nil", order.ReceivedAt())
	}
}

func TestPurchaseOrder_Cancel_FirstCall_Succeeds(t *testing.T) {
	// Arrange
	order := newTestPurchaseOrder(t)
	beforeVersion := order.Version()

	// Act
	err := order.Cancel()

	// Assert
	if err != nil {
		t.Fatalf("Cancel returned unexpected error: %v", err)
	}
	if order.Status() != domain.PurchaseOrderStatusCancelado {
		t.Errorf("order.Status() = %v, want %v", order.Status(), domain.PurchaseOrderStatusCancelado)
	}
	if order.Version() != beforeVersion+1 {
		t.Errorf("order.Version() = %d, want %d", order.Version(), beforeVersion+1)
	}
}

func TestPurchaseOrder_Cancel_OnReceivedOrder_ReturnsConflictAndLeavesStateIntact(t *testing.T) {
	// Arrange
	order := newTestPurchaseOrder(t)
	if err := order.MarkAsReceived(time.Now().UTC()); err != nil {
		t.Fatalf("MarkAsReceived returned unexpected error: %v", err)
	}
	beforeVersion := order.Version()

	// Act
	err := order.Cancel()

	// Assert
	if err == nil {
		t.Fatal("Cancel on a received order returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
	if order.Status() != domain.PurchaseOrderStatusRecebido {
		t.Errorf("order.Status() = %v after failed Cancel, want unchanged %v", order.Status(), domain.PurchaseOrderStatusRecebido)
	}
	if order.Version() != beforeVersion {
		t.Errorf("order.Version() = %d after failed Cancel, want unchanged %d", order.Version(), beforeVersion)
	}
}

func TestPurchaseOrder_Cancel_TwiceInARow_SecondCallReturnsConflictAndLeavesStateIntact(t *testing.T) {
	// Arrange
	order := newTestPurchaseOrder(t)
	if err := order.Cancel(); err != nil {
		t.Fatalf("first Cancel returned unexpected error: %v", err)
	}
	beforeVersion := order.Version()

	// Act
	err := order.Cancel()

	// Assert
	if err == nil {
		t.Fatal("second Cancel returned nil error, want a Conflict error")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Conflict)
	}
	if order.Version() != beforeVersion {
		t.Errorf("order.Version() = %d after second Cancel, want unchanged %d", order.Version(), beforeVersion)
	}
}
