package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestApplyDiscount_Valid_UpdatesProductAndPublishesEvent(t *testing.T) {
	// Arrange
	product := mustFakeProduct(t)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)
	now := time.Now().UTC()

	// Act
	updated, err := application.ApplyDiscount(context.Background(), uow, application.ApplyDiscountInput{
		ProductID: product.ID(),
		Discount:  mustFakeDiscountPercent(t, 2500), // 25%
	}, now, "user-1")

	// Assert
	if err != nil {
		t.Fatalf("ApplyDiscount: unexpected error: %v", err)
	}
	if updated.CurrentPrice().Cents() != 1125 { // 1500 * 0.75
		t.Errorf("updated.CurrentPrice().Cents() = %d, want 1125", updated.CurrentPrice().Cents())
	}
	snapshot := uow.snapshot()
	persisted := snapshot.products[product.ID()]
	if persisted.Version() != 2 {
		t.Errorf("persisted.Version() = %d, want 2", persisted.Version())
	}
	if len(snapshot.events) != 1 {
		t.Fatalf("events published = %d, want 1", len(snapshot.events))
	}
	if len(snapshot.auditEntries) != 1 {
		t.Fatalf("audit entries recorded = %d, want 1", len(snapshot.auditEntries))
	}
	entry := snapshot.auditEntries[0]
	if entry.ActorID != "user-1" {
		t.Errorf("entry.ActorID = %q, want %q", entry.ActorID, "user-1")
	}
	if entry.Action != "product.discount_applied" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "product.discount_applied")
	}
	if entry.EntityID != product.ID().String() {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, product.ID().String())
	}
}

func TestApplyDiscount_ProductNotFound_ReturnsNotFoundError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())

	// Act
	_, err := application.ApplyDiscount(context.Background(), uow, application.ApplyDiscountInput{
		ProductID: domain.NewID(),
		Discount:  mustFakeDiscountPercent(t, 1000),
	}, time.Now().UTC(), "")

	// Assert
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.NotFound)
	}
}

// TestApplyDiscount_WouldZeroOutPrice_LeavesPersistedStateUntouched proves
// this module's core price invariant through the application layer: a
// rejected discount must persist nothing.
func TestApplyDiscount_WouldZeroOutPrice_LeavesPersistedStateUntouched(t *testing.T) {
	// Arrange
	product := mustFakeProduct(t)
	state := newFakeState()
	state.products[product.ID()] = product
	uow := newFakeUnitOfWork(state)

	// Act
	_, err := application.ApplyDiscount(context.Background(), uow, application.ApplyDiscountInput{
		ProductID: product.ID(),
		Discount:  mustFakeDiscountPercent(t, 10000), // 100%
	}, time.Now().UTC(), "user-1")

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	snapshot := uow.snapshot()
	persisted := snapshot.products[product.ID()]
	if persisted.Version() != 1 {
		t.Errorf("persisted.Version() = %d after rejected discount, want unchanged 1", persisted.Version())
	}
	if len(snapshot.auditEntries) != 0 {
		t.Errorf("audit entries = %v after rejected discount, want none: the invariant fails BEFORE any write is attempted", snapshot.auditEntries)
	}
	if persisted.Discount() != nil {
		t.Errorf("persisted.Discount() = %v after rejected discount, want nil", persisted.Discount())
	}
	if len(snapshot.events) != 0 {
		t.Errorf("events = %v, want empty after rejected discount", snapshot.events)
	}
}
