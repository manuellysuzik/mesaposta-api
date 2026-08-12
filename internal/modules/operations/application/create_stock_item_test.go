package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestCreateStockItem_Valid_PersistsItem(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateStockItemInput{
		TenantID:    mustFakeUOWTenantID(t),
		Name:        "Farinha de trigo",
		Unit:        domain.StockUnitKg,
		Quantity:    mustFakeQuantity(t, 0),
		MinQuantity: mustFakeQuantity(t, 1000),
		CostPerUnit: mustMoneyFor(t, 500),
	}

	// Act
	item, err := application.CreateStockItem(context.Background(), uow, input)

	// Assert
	if err != nil {
		t.Fatalf("CreateStockItem: unexpected error: %v", err)
	}
	snapshot := uow.snapshot()
	persisted, ok := snapshot.stockItems[item.ID()]
	if !ok {
		t.Fatal("stock item was not persisted")
	}
	if persisted.Name() != "Farinha de trigo" {
		t.Errorf("persisted name = %q, want %q", persisted.Name(), "Farinha de trigo")
	}
}

func TestCreateStockItem_InvalidName_PersistsNothing(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	input := application.CreateStockItemInput{
		TenantID:    mustFakeUOWTenantID(t),
		Name:        "",
		Unit:        domain.StockUnitKg,
		Quantity:    mustFakeQuantity(t, 0),
		MinQuantity: mustFakeQuantity(t, 0),
		CostPerUnit: mustMoneyFor(t, 0),
	}

	// Act
	_, err := application.CreateStockItem(context.Background(), uow, input)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if snapshot := uow.snapshot(); len(snapshot.stockItems) != 0 {
		t.Errorf("stockItems = %v, want empty", snapshot.stockItems)
	}
}
