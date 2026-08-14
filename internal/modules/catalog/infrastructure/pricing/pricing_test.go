package pricing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/infrastructure/pricing"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const pricingTestTenantUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(pricingTestTenantUUID)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	return id
}

func mustProduct(t *testing.T, name string, priceCents int64) *domain.Product {
	t.Helper()
	price, err := domain.NewMoney(priceCents)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), name, "", price, nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return &product
}

// --- fake domain.UnitOfWork, scoped to this test file only -------------

type fakeUnitOfWork struct {
	products map[domain.ID]*domain.Product
	findErr  error
}

var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

func (f *fakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	return fn(ctx, &fakeRepositories{products: f.products, findErr: f.findErr})
}

type fakeRepositories struct {
	products map[domain.ID]*domain.Product
	findErr  error
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (r *fakeRepositories) Products() domain.ProductRepository {
	return &fakeProductRepository{products: r.products, findErr: r.findErr}
}
func (r *fakeRepositories) Recipes() domain.RecipeRepository    { return nil }
func (r *fakeRepositories) Events() outbox.DomainEventPublisher { return nil }
func (r *fakeRepositories) Audit() audit.Recorder               { return nil }

type fakeProductRepository struct {
	products map[domain.ID]*domain.Product
	findErr  error
}

var _ domain.ProductRepository = (*fakeProductRepository)(nil)

func (r *fakeProductRepository) FindByID(_ context.Context, id domain.ID) (*domain.Product, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	product, ok := r.products[id]
	if !ok {
		return nil, errs.New(errs.NotFound, "product_not_found", "product not found", nil)
	}
	return product, nil
}
func (r *fakeProductRepository) FindActive(_ context.Context) ([]*domain.Product, error) {
	return nil, nil
}
func (r *fakeProductRepository) Insert(_ context.Context, _ *domain.Product) error { return nil }
func (r *fakeProductRepository) UpdateWithVersion(_ context.Context, _ *domain.Product, _ int64) error {
	return nil
}

// --- tests ---------------------------------------------------------------

func TestGetProductPrice_Found_ReturnsCurrentPriceInCents(t *testing.T) {
	// Arrange
	product := mustProduct(t, "X-Burguer", 1500)
	discount, err := domain.NewDiscountPercent(1000) // 10%
	if err != nil {
		t.Fatalf("NewDiscountPercent: %v", err)
	}
	if err := product.ApplyDiscount(discount); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	uow := &fakeUnitOfWork{products: map[domain.ID]*domain.Product{product.ID(): product}}
	reader := pricing.NewProductPriceReader(uow)

	// Act
	got, err := reader.GetProductPrice(context.Background(), product.ID().String())

	// Assert
	if err != nil {
		t.Fatalf("GetProductPrice: unexpected error: %v", err)
	}
	if got.ProductID != product.ID().String() {
		t.Errorf("ProductID = %q, want %q", got.ProductID, product.ID().String())
	}
	wantCents := product.CurrentPrice().Cents()
	if got.PriceCents != wantCents {
		t.Errorf("PriceCents = %d, want %d (current, discounted price -- not base price %d)",
			got.PriceCents, wantCents, product.BasePrice().Cents())
	}
	if !got.IsActive {
		t.Errorf("IsActive = false, want true")
	}
}

func TestGetProductPrice_NoDiscount_PriceCentsEqualsBasePrice(t *testing.T) {
	product := mustProduct(t, "Refrigerante", 700)
	uow := &fakeUnitOfWork{products: map[domain.ID]*domain.Product{product.ID(): product}}
	reader := pricing.NewProductPriceReader(uow)

	got, err := reader.GetProductPrice(context.Background(), product.ID().String())
	if err != nil {
		t.Fatalf("GetProductPrice: unexpected error: %v", err)
	}
	if got.PriceCents != 700 {
		t.Errorf("PriceCents = %d, want 700", got.PriceCents)
	}
}

func TestGetProductPrice_MalformedID_ReturnsInvalidKindError(t *testing.T) {
	uow := &fakeUnitOfWork{products: map[domain.ID]*domain.Product{}}
	reader := pricing.NewProductPriceReader(uow)

	_, err := reader.GetProductPrice(context.Background(), "not-a-valid-uuid")
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestGetProductPrice_UnknownID_ReturnsNotFoundKindError(t *testing.T) {
	uow := &fakeUnitOfWork{products: map[domain.ID]*domain.Product{}}
	reader := pricing.NewProductPriceReader(uow)

	_, err := reader.GetProductPrice(context.Background(), domain.NewID().String())
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestGetProductPrice_RepositoryError_PropagatesUnchanged(t *testing.T) {
	sentinel := errors.New("boom")
	uow := &fakeUnitOfWork{findErr: sentinel}
	reader := pricing.NewProductPriceReader(uow)

	_, err := reader.GetProductPrice(context.Background(), domain.NewID().String())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap sentinel %v", err, sentinel)
	}
}
