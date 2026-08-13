package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const orderTestTenantUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustOrderTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(orderTestTenantUUID)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	return id
}

func mustOrderItem(t *testing.T, productID string, count int64, cents int64) domain.OrderItem {
	t.Helper()
	item, err := domain.NewOrderItem(productID, mustOrderItemQuantity(t, count), mustOrderItemMoney(t, cents))
	if err != nil {
		t.Fatalf("NewOrderItem: %v", err)
	}
	return item
}

func mustOrder(t *testing.T, items ...domain.OrderItem) domain.Order {
	t.Helper()
	order, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.NewID(), nil, nil, items)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return order
}

func TestNewOrder_ZeroTableID_ReturnsInvalid(t *testing.T) {
	items := []domain.OrderItem{mustOrderItem(t, "p1", 1, 100)}
	_, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.ID{}, nil, nil, items)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrder_NoItems_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.NewID(), nil, nil, nil)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrder_DuplicateProduct_ReturnsInvalid(t *testing.T) {
	items := []domain.OrderItem{
		mustOrderItem(t, "p1", 1, 100),
		mustOrderItem(t, "p1", 2, 100),
	}
	_, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.NewID(), nil, nil, items)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrder_BlankCustomerName_ReturnsInvalid(t *testing.T) {
	blank := "   "
	items := []domain.OrderItem{mustOrderItem(t, "p1", 1, 100)}
	_, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.NewID(), &blank, nil, items)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrder_BlankCustomerPhone_ReturnsInvalid(t *testing.T) {
	blank := ""
	items := []domain.OrderItem{mustOrderItem(t, "p1", 1, 100)}
	_, err := domain.NewOrder(domain.NewID(), mustOrderTenantID(t), domain.NewID(), nil, &blank, items)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrder_Valid_StartsAtPendenteConfirmacaoVersion1(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 2, 1500))

	if order.Status() != domain.OrderStatusPendenteConfirmacao {
		t.Errorf("Status() = %q, want %q", order.Status(), domain.OrderStatusPendenteConfirmacao)
	}
	if order.Version() != 1 {
		t.Errorf("Version() = %d, want 1", order.Version())
	}
}

func TestOrder_Items_DefensiveCopy(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 2, 1500))

	items := order.Items()
	items[0] = mustOrderItem(t, "p2", 99, 1)

	if order.Items()[0].ProductID() != "p1" {
		t.Error("mutating the slice returned by Items() reached back into the aggregate")
	}
}

func TestOrder_Total(t *testing.T) {
	order := mustOrder(t,
		mustOrderItem(t, "p1", 2, 1500), // 3000
		mustOrderItem(t, "p2", 1, 700),  // 700
	)

	total, err := order.Total()
	if err != nil {
		t.Fatalf("Total: unexpected error: %v", err)
	}
	if total.Cents() != 3700 {
		t.Errorf("Total().Cents() = %d, want 3700", total.Cents())
	}
}

// TestOrder_StateMachine_HappyPath walks the full
// pendente_confirmacao -> confirmado -> em_preparo -> pronto -> entregue
// path, asserting Version increments by exactly 1 at each step.
func TestOrder_StateMachine_HappyPath(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))

	steps := []struct {
		name   string
		call   func() error
		status domain.OrderStatus
	}{
		{"Confirm", order.Confirm, domain.OrderStatusConfirmado},
		{"StartPreparing", order.StartPreparing, domain.OrderStatusEmPreparo},
		{"MarkReady", order.MarkReady, domain.OrderStatusPronto},
		{"MarkDelivered", order.MarkDelivered, domain.OrderStatusEntregue},
	}

	wantVersion := int64(1)
	for _, step := range steps {
		if err := step.call(); err != nil {
			t.Fatalf("%s: unexpected error: %v", step.name, err)
		}
		wantVersion++
		if order.Status() != step.status {
			t.Errorf("%s: Status() = %q, want %q", step.name, order.Status(), step.status)
		}
		if order.Version() != wantVersion {
			t.Errorf("%s: Version() = %d, want %d", step.name, order.Version(), wantVersion)
		}
	}
}

func TestOrder_Cancel_FromEveryNonTerminalStatus(t *testing.T) {
	tests := []struct {
		name    string
		advance func(o *domain.Order) error
	}{
		{"from pendente_confirmacao", func(o *domain.Order) error { return nil }},
		{"from confirmado", func(o *domain.Order) error { return o.Confirm() }},
		{"from em_preparo", func(o *domain.Order) error {
			if err := o.Confirm(); err != nil {
				return err
			}
			return o.StartPreparing()
		}},
		{"from pronto", func(o *domain.Order) error {
			if err := o.Confirm(); err != nil {
				return err
			}
			if err := o.StartPreparing(); err != nil {
				return err
			}
			return o.MarkReady()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
			if err := tt.advance(&order); err != nil {
				t.Fatalf("test setup: %v", err)
			}
			versionBeforeCancel := order.Version()

			if err := order.Cancel(); err != nil {
				t.Fatalf("Cancel: unexpected error: %v", err)
			}
			if order.Status() != domain.OrderStatusCancelado {
				t.Errorf("Status() = %q, want %q", order.Status(), domain.OrderStatusCancelado)
			}
			if order.Version() != versionBeforeCancel+1 {
				t.Errorf("Version() = %d, want %d", order.Version(), versionBeforeCancel+1)
			}
		})
	}
}

func TestOrder_Cancel_FromEntregue_ReturnsConflictAndLeavesUntouched(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
	if err := order.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := order.StartPreparing(); err != nil {
		t.Fatalf("StartPreparing: %v", err)
	}
	if err := order.MarkReady(); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if err := order.MarkDelivered(); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}

	statusBefore := order.Status()
	versionBefore := order.Version()

	err := order.Cancel()
	if !errs.IsKind(err, errs.Conflict) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
	if order.Status() != statusBefore {
		t.Errorf("Status() = %q after rejected Cancel, want unchanged %q", order.Status(), statusBefore)
	}
	if order.Version() != versionBefore {
		t.Errorf("Version() = %d after rejected Cancel, want unchanged %d", order.Version(), versionBefore)
	}
}

func TestOrder_Cancel_AlreadyCancelled_ReturnsConflict(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
	if err := order.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	err := order.Cancel()
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestOrder_Confirm_AlreadyConfirmed_ReturnsConflictAndLeavesUntouched(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
	if err := order.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	versionBefore := order.Version()

	err := order.Confirm()
	if !errs.IsKind(err, errs.Conflict) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
	if order.Version() != versionBefore {
		t.Errorf("Version() = %d after rejected Confirm, want unchanged %d", order.Version(), versionBefore)
	}
}

func TestOrder_StartPreparing_WithoutConfirm_ReturnsConflict(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))

	err := order.StartPreparing()
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestOrder_MarkReady_WithoutStartPreparing_ReturnsConflict(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
	if err := order.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	err := order.MarkReady()
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestOrder_MarkDelivered_WithoutMarkReady_ReturnsConflict(t *testing.T) {
	order := mustOrder(t, mustOrderItem(t, "p1", 1, 100))
	if err := order.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := order.StartPreparing(); err != nil {
		t.Fatalf("StartPreparing: %v", err)
	}

	err := order.MarkDelivered()
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}
