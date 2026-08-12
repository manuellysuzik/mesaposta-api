package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreatePurchaseOrder_Valid_PersistsOrder(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	item1, err := domain.NewPurchaseOrderItem(domain.NewID(), mustFakeQuantity(t, 1000), mustMoneyFor(t, 500))
	if err != nil {
		t.Fatalf("NewPurchaseOrderItem: %v", err)
	}
	input := application.CreatePurchaseOrderInput{
		TenantID:   mustFakeUOWTenantID(t),
		SupplierID: domain.NewID(),
		Items:      []domain.PurchaseOrderItem{item1},
	}

	// Act
	order, err := application.CreatePurchaseOrder(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.purchaseOrders[order.ID()]
	if !ok {
		t.Fatal("purchase order was not persisted")
	}
	if persisted.Status() != domain.PurchaseOrderStatusPendente {
		t.Errorf("persisted status = %v, want %v", persisted.Status(), domain.PurchaseOrderStatusPendente)
	}
}

func TestCreatePurchaseOrder_NoItems_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreatePurchaseOrderInput{
		TenantID:   mustFakeUOWTenantID(t),
		SupplierID: domain.NewID(),
		Items:      nil,
	}

	// Act
	_, err := application.CreatePurchaseOrder(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.purchaseOrders) != 0 {
		t.Errorf("purchaseOrders = %v, want empty", snapshot.purchaseOrders)
	}
}
