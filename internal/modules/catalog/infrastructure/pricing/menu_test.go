package pricing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/pricing"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// mustMenuUnitOfWork wraps a fakeUnitOfWork (pricing_test.go) whose
// Products().FindActive method returns the given products directly --
// overriding this package's shared fake's own FindActive stub (which
// always returns nil, nil), so MenuReader's own mapping logic (not
// FindActive's filtering, already proven by
// internal/modules/catalog/application/list_active_products_test.go) is
// what this file actually exercises.
type menuFakeUnitOfWork struct {
	products []*domain.Product
	err      error
}

func (u *menuFakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	return fn(ctx, &menuFakeRepositories{products: u.products, err: u.err})
}

type menuFakeRepositories struct {
	products []*domain.Product
	err      error
}

func (r *menuFakeRepositories) Products() domain.ProductRepository {
	return &menuFakeProductRepository{products: r.products, err: r.err}
}
func (r *menuFakeRepositories) Recipes() domain.RecipeRepository    { return nil }
func (r *menuFakeRepositories) Events() outbox.DomainEventPublisher { return nil }
func (r *menuFakeRepositories) Audit() audit.Recorder               { return nil }

type menuFakeProductRepository struct {
	products []*domain.Product
	err      error
}

func (r *menuFakeProductRepository) FindByID(_ context.Context, _ domain.ID) (*domain.Product, error) {
	return nil, nil
}
func (r *menuFakeProductRepository) FindActive(_ context.Context) ([]*domain.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.products, nil
}
func (r *menuFakeProductRepository) Insert(_ context.Context, _ *domain.Product) error { return nil }
func (r *menuFakeProductRepository) UpdateWithVersion(_ context.Context, _ *domain.Product, _ int64) error {
	return nil
}

func TestListActiveMenu_MapsProductsToMenuItemsWithCurrentPrice(t *testing.T) {
	product := mustProduct(t, "X-Burguer", 1500)
	discount, err := domain.NewDiscountPercent(1000) // 10%
	if err != nil {
		t.Fatalf("NewDiscountPercent: %v", err)
	}
	if err := product.ApplyDiscount(discount); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	reader := pricing.NewMenuReader(&menuFakeUnitOfWork{products: []*domain.Product{product}})

	items, err := reader.ListActiveMenu(context.Background())
	if err != nil {
		t.Fatalf("ListActiveMenu: unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ProductID != product.ID().String() {
		t.Errorf("ProductID = %q, want %q", items[0].ProductID, product.ID().String())
	}
	if items[0].Name != "X-Burguer" {
		t.Errorf("Name = %q, want %q", items[0].Name, "X-Burguer")
	}
	wantCents := product.CurrentPrice().Cents()
	if items[0].PriceCents != wantCents {
		t.Errorf("PriceCents = %d, want %d (current, discounted price)", items[0].PriceCents, wantCents)
	}
}

func TestListActiveMenu_Empty_ReturnsEmptySlice(t *testing.T) {
	reader := pricing.NewMenuReader(&menuFakeUnitOfWork{})

	items, err := reader.ListActiveMenu(context.Background())
	if err != nil {
		t.Fatalf("ListActiveMenu: unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestListActiveMenu_RepositoryError_PropagatesUnchanged(t *testing.T) {
	sentinel := errors.New("boom")
	reader := pricing.NewMenuReader(&menuFakeUnitOfWork{err: sentinel})

	_, err := reader.ListActiveMenu(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap sentinel %v", err, sentinel)
	}
}
