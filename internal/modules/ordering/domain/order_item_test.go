package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustOrderItemQuantity(t *testing.T, count int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(count)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", count, err)
	}
	return q
}

func mustOrderItemMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

func TestNewOrderItem_EmptyProductID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewOrderItem("", mustOrderItemQuantity(t, 1), mustOrderItemMoney(t, 100))
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewOrderItem_Valid(t *testing.T) {
	item, err := domain.NewOrderItem("product-1", mustOrderItemQuantity(t, 3), mustOrderItemMoney(t, 1500))
	if err != nil {
		t.Fatalf("NewOrderItem: unexpected error: %v", err)
	}
	if item.ProductID() != "product-1" {
		t.Errorf("ProductID() = %q, want %q", item.ProductID(), "product-1")
	}
	if item.Quantity().Count() != 3 {
		t.Errorf("Quantity().Count() = %d, want 3", item.Quantity().Count())
	}
	if item.UnitPrice().Cents() != 1500 {
		t.Errorf("UnitPrice().Cents() = %d, want 1500", item.UnitPrice().Cents())
	}
}

func TestOrderItem_LineTotal(t *testing.T) {
	item, err := domain.NewOrderItem("product-1", mustOrderItemQuantity(t, 3), mustOrderItemMoney(t, 1500))
	if err != nil {
		t.Fatalf("NewOrderItem: unexpected error: %v", err)
	}

	total, err := item.LineTotal()
	if err != nil {
		t.Fatalf("LineTotal: unexpected error: %v", err)
	}
	if total.Cents() != 4500 {
		t.Errorf("LineTotal().Cents() = %d, want 4500", total.Cents())
	}
}
