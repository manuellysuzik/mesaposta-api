package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// RegisterStockMovementInput carries everything RegisterStockMovement needs
// to register a single movement against one StockItem.
type RegisterStockMovementInput struct {
	StockItemID  domain.ID
	MovementType domain.StockMovementType
	Quantity     domain.Quantity
	Reason       string
}

// stockMovementQuantityScale converts a Quantity's int64-thousandths
// representation into a float64 whole-unit amount, matching the scale
// contracts.StockItemMovementRegisteredPayload.QuantityDelta is documented
// to carry -- the same conversion domain.ReceivePurchaseOrder applies
// internally (see domain/receiving.go's thousandthsToUnits), duplicated
// here in application/ since that helper is unexported and this package
// cannot reach into domain/'s internals for it.
const stockMovementQuantityScale = 1000.0

// RegisterStockMovement is the application-layer use case that records a
// single stock movement (an "entrada" or "saida") against one StockItem,
// updating that item's quantity on hand and publishing a domain event, all
// atomically within a single UnitOfWork.Do call.
//
// For a "saida" (outbound) movement, RegisterStockMovement calls
// StockItem.Withdraw: if the requested quantity exceeds the item's
// quantity on hand, Withdraw returns an *errs.Error of Kind errs.Conflict,
// RegisterStockMovement propagates it unchanged, and -- because that
// failure happens before ANY repository write is attempted -- Do rolls
// back with nothing persisted: neither the StockItem nor a StockMovement.
// This is acceptance criterion 1 (insufficient stock rejects the whole
// operation) proved once more, this time through the application layer
// rather than directly against the domain aggregate.
//
// For an "entrada" (inbound) movement, RegisterStockMovement calls
// StockItem.Receive, then persists the updated StockItem and the new
// StockMovement, and publishes a
// contracts.EventStockItemMovementRegistered event, all within the same Do
// call.
func RegisterStockMovement(ctx context.Context, uow domain.UnitOfWork, input RegisterStockMovementInput) error {
	if !input.MovementType.Valid() {
		return errs.New(errs.Invalid, "invalid_stock_movement_type",
			"stock movement type is not one of the known values", nil)
	}

	return uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		item, err := r.StockItems().FindByID(ctx, input.StockItemID)
		if err != nil {
			return err
		}
		expectedVersion := item.Version()

		if err := applyStockMovementQuantity(item, input); err != nil {
			return err
		}

		movement, err := domain.NewStockMovement(
			domain.NewID(),
			item.TenantID(),
			item.ID(),
			input.MovementType,
			input.Quantity,
			input.Reason,
		)
		if err != nil {
			return err
		}

		if err := r.StockItems().UpdateWithVersion(ctx, item, expectedVersion); err != nil {
			return err
		}

		if err := r.StockMovements().Insert(ctx, movement); err != nil {
			return err
		}

		event, err := stockMovementEvent(item, movement)
		if err != nil {
			return err
		}

		return r.Events().Publish(ctx, event)
	})
}

// applyStockMovementQuantity mutates item in place according to
// input.MovementType: Withdraw for "saida", Receive for "entrada".
func applyStockMovementQuantity(item *domain.StockItem, input RegisterStockMovementInput) error {
	switch input.MovementType {
	case domain.StockMovementTypeSaida:
		return item.Withdraw(input.Quantity)
	case domain.StockMovementTypeEntrada:
		return item.Receive(input.Quantity)
	default:
		// Unreachable: RegisterStockMovement validates input.MovementType
		// before calling uow.Do at all.
		return errs.New(errs.Invalid, "invalid_stock_movement_type",
			"stock movement type is not one of the known values", nil)
	}
}

// stockMovementEvent builds the EventStockItemMovementRegistered domain
// event for movement, with QuantityDelta signed to match
// StockItemMovementRegisteredPayload's documented convention: positive for
// inbound movements, negative for outbound ones.
func stockMovementEvent(item *domain.StockItem, movement domain.StockMovement) (outbox.DomainEvent, error) {
	delta := float64(movement.Quantity().Thousandths()) / stockMovementQuantityScale
	if movement.MovementType() == domain.StockMovementTypeSaida {
		delta = -delta
	}

	return outbox.NewDomainEvent(
		item.TenantID(),
		contracts.EventStockItemMovementRegistered,
		item.ID().String(),
		movement.CreatedAt(),
		contracts.StockItemMovementRegisteredPayload{
			StockItemID:   item.ID().String(),
			MovementID:    movement.ID().String(),
			QuantityDelta: delta,
			OccurredAt:    movement.CreatedAt(),
		},
	)
}
