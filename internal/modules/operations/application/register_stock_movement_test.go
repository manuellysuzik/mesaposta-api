package application_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func newStockItemFixture(t *testing.T, quantityThousandths int64) (*fakeUnitOfWork, *domain.StockItem) {
	t.Helper()
	item, err := domain.NewStockItem(
		domain.NewID(),
		mustFakeUOWTenantID(t),
		"Farinha de trigo",
		domain.StockUnitKg,
		mustFakeQuantity(t, quantityThousandths),
		mustFakeQuantity(t, 0),
		mustMoneyFor(t, 500),
		nil,
	)
	if err != nil {
		t.Fatalf("NewStockItem: %v", err)
	}
	state := newFakeState()
	state.stockItems[item.ID()] = &item
	return newFakeUnitOfWork(state), &item
}

func TestRegisterStockMovement_Saida_InsufficientStock_PersistsNothing(t *testing.T) {
	// Arrange: 1000 thousandths on hand, try to withdraw 5000.
	uow, item := newStockItemFixture(t, 1000)

	// Act
	err := application.RegisterStockMovement(context.Background(), uow, application.RegisterStockMovementInput{
		StockItemID:  item.ID(),
		MovementType: domain.StockMovementTypeSaida,
		Quantity:     mustFakeQuantity(t, 5000),
		Reason:       "Venda de prato",
	})

	// Assert
	if err == nil {
		t.Fatal("RegisterStockMovement: expected an error, got nil")
	}
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Conflict)
	}

	snapshot := uow.snapshot()
	if got := snapshot.stockItems[item.ID()].Quantity().Thousandths(); got != 1000 {
		t.Errorf("stock item quantity = %d, want unchanged 1000", got)
	}
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty", snapshot.events)
	}
}

func TestRegisterStockMovement_Saida_SufficientStock_PersistsItemAndMovement(t *testing.T) {
	// Arrange
	uow, item := newStockItemFixture(t, 5000)

	// Act
	err := application.RegisterStockMovement(context.Background(), uow, application.RegisterStockMovementInput{
		StockItemID:  item.ID(),
		MovementType: domain.StockMovementTypeSaida,
		Quantity:     mustFakeQuantity(t, 2000),
		Reason:       "Venda de prato",
	})

	// Assert
	if err != nil {
		t.Fatalf("RegisterStockMovement: unexpected error: %v", err)
	}

	snapshot := uow.snapshot()
	if got := snapshot.stockItems[item.ID()].Quantity().Thousandths(); got != 3000 {
		t.Errorf("stock item quantity = %d, want 3000", got)
	}
	if len(snapshot.stockMovements) != 1 {
		t.Fatalf("len(stockMovements) = %d, want 1", len(snapshot.stockMovements))
	}
	if snapshot.stockMovements[0].MovementType() != domain.StockMovementTypeSaida {
		t.Errorf("movement type = %v, want %v", snapshot.stockMovements[0].MovementType(), domain.StockMovementTypeSaida)
	}
	if len(snapshot.events) != 1 {
		t.Errorf("len(events) = %d, want 1", len(snapshot.events))
	}
}

func TestRegisterStockMovement_Entrada_PersistsItemAndMovement(t *testing.T) {
	// Arrange
	uow, item := newStockItemFixture(t, 1000)

	// Act
	err := application.RegisterStockMovement(context.Background(), uow, application.RegisterStockMovementInput{
		StockItemID:  item.ID(),
		MovementType: domain.StockMovementTypeEntrada,
		Quantity:     mustFakeQuantity(t, 4000),
		Reason:       "Ajuste de inventario",
	})

	// Assert
	if err != nil {
		t.Fatalf("RegisterStockMovement: unexpected error: %v", err)
	}

	snapshot := uow.snapshot()
	if got := snapshot.stockItems[item.ID()].Quantity().Thousandths(); got != 5000 {
		t.Errorf("stock item quantity = %d, want 5000", got)
	}
	if len(snapshot.stockMovements) != 1 {
		t.Fatalf("len(stockMovements) = %d, want 1", len(snapshot.stockMovements))
	}
	if len(snapshot.events) != 1 {
		t.Errorf("len(events) = %d, want 1", len(snapshot.events))
	}
}

func TestRegisterStockMovement_UnknownStockItem_ReturnsNotFound(t *testing.T) {
	// Arrange
	uow, _ := newStockItemFixture(t, 1000)

	// Act
	err := application.RegisterStockMovement(context.Background(), uow, application.RegisterStockMovementInput{
		StockItemID:  domain.NewID(),
		MovementType: domain.StockMovementTypeEntrada,
		Quantity:     mustFakeQuantity(t, 1000),
		Reason:       "Ajuste de inventario",
	})

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}

func TestRegisterStockMovement_InvalidMovementType_ReturnsInvalid(t *testing.T) {
	// Arrange
	uow, item := newStockItemFixture(t, 1000)

	// Act
	err := application.RegisterStockMovement(context.Background(), uow, application.RegisterStockMovementInput{
		StockItemID:  item.ID(),
		MovementType: domain.StockMovementType("invalida"),
		Quantity:     mustFakeQuantity(t, 1000),
		Reason:       "Ajuste de inventario",
	})

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	snapshot := uow.snapshot()
	if len(snapshot.stockMovements) != 0 {
		t.Errorf("stockMovements = %v, want empty", snapshot.stockMovements)
	}
}
