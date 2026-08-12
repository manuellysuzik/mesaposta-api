package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This file exercises fakeUnitOfWork (fakes_test.go) directly, with no
// dependency on any real use case -- proving the fake itself genuinely
// simulates a database transaction's rollback semantics BEFORE any test in
// this package leans on that behavior to prove atomicity of a real use
// case. Without this guarantee, every "nothing was persisted after an
// error" assertion elsewhere in this package would be testing nothing at
// all.

const fakeUOWTestTenantID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustFakeUOWTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(fakeUOWTestTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", fakeUOWTestTenantID, err)
	}
	return id
}

func mustFakeQuantity(t *testing.T, thousandths int64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(thousandths)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", thousandths, err)
	}
	return q
}

func mustFakeMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", cents, err)
	}
	return m
}

func mustFakeDiscountPercent(t *testing.T, hundredths int64) domain.DiscountPercent {
	t.Helper()
	d, err := domain.NewDiscountPercent(hundredths)
	if err != nil {
		t.Fatalf("NewDiscountPercent(%d): %v", hundredths, err)
	}
	return d
}

func mustFakeProduct(t *testing.T) *domain.Product {
	t.Helper()
	product, err := domain.NewProduct(
		domain.NewID(), mustFakeUOWTenantID(t), "X-Burguer", "Hambúrguer artesanal", mustFakeMoney(t, 1500), nil,
	)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return &product
}

func mustFakeProductNamed(t *testing.T, name string) *domain.Product {
	t.Helper()
	product, err := domain.NewProduct(
		domain.NewID(), mustFakeUOWTenantID(t), name, "", mustFakeMoney(t, 700), nil,
	)
	if err != nil {
		t.Fatalf("NewProduct(%q): %v", name, err)
	}
	return &product
}

func mustFakeRecipe(t *testing.T, ingredients []domain.RecipeIngredient) *domain.Recipe {
	t.Helper()
	recipe, err := domain.NewRecipe(domain.NewID(), mustFakeUOWTenantID(t), "Molho especial", "Receita da casa", ingredients)
	if err != nil {
		t.Fatalf("NewRecipe: %v", err)
	}
	return &recipe
}

func TestFakeUnitOfWork_Do_CommitsWritesOnSuccess(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	product := mustFakeProduct(t)

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		return r.Products().Insert(ctx, product)
	})

	// Assert
	if err != nil {
		t.Fatalf("Do: unexpected error: %v", err)
	}
	got, ok := uow.snapshot().products[product.ID()]
	if !ok {
		t.Fatal("Do: product was not committed to real state after fn returned nil")
	}
	if got.ID() != product.ID() {
		t.Errorf("committed product ID = %v, want %v", got.ID(), product.ID())
	}
}

func TestFakeUnitOfWork_Do_RollsBackOnError(t *testing.T) {
	// Arrange
	uow := newFakeUnitOfWork(newFakeState())
	product := mustFakeProduct(t)
	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		if insertErr := r.Products().Insert(ctx, product); insertErr != nil {
			return insertErr
		}
		// A second write, past the first, to prove EVERYTHING attempted
		// within this failed Do call is discarded -- not just the last
		// write before the error.
		recipe := mustFakeRecipe(t, nil)
		if insertErr := r.Recipes().Insert(ctx, recipe); insertErr != nil {
			return insertErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	snapshot := uow.snapshot()
	if len(snapshot.products) != 0 {
		t.Errorf("Do: products = %v, want empty after rollback", snapshot.products)
	}
	if len(snapshot.recipes) != 0 {
		t.Errorf("Do: recipes = %v, want empty after rollback", snapshot.recipes)
	}
}

func TestFakeUnitOfWork_Do_RollbackLeavesPreExistingStateUntouched(t *testing.T) {
	// Arrange: seed the fake with one already-committed product, then run a
	// failing Do call that tries to mutate it.
	seed := mustFakeProduct(t)
	state := newFakeState()
	state.products[seed.ID()] = seed
	uow := newFakeUnitOfWork(state)

	sentinel := errors.New("boom")

	// Act
	err := uow.Do(context.Background(), func(ctx context.Context, r domain.Repositories) error {
		mutated := *seed
		if applyErr := mutated.ApplyDiscount(mustFakeDiscountPercent(t, 1000)); applyErr != nil {
			return applyErr
		}
		if updateErr := r.Products().UpdateWithVersion(ctx, &mutated, seed.Version()); updateErr != nil {
			return updateErr
		}
		return sentinel
	})

	// Assert
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do: err = %v, want sentinel %v", err, sentinel)
	}
	got := uow.snapshot().products[seed.ID()]
	if got.Version() != seed.Version() {
		t.Errorf("Do: version = %d after rollback, want unchanged %d", got.Version(), seed.Version())
	}
	if got.Discount() != nil {
		t.Errorf("Do: discount = %v after rollback, want unchanged nil", got.Discount())
	}
}
