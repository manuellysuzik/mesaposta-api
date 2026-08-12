package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const testTenantIDStr = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(testTenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", testTenantIDStr, err)
	}
	return id
}

func mustProductIngredient(t *testing.T, ingredientID domain.ID, quantity domain.Quantity) domain.ProductIngredient {
	t.Helper()
	ing, err := domain.NewProductIngredient(ingredientID, quantity)
	if err != nil {
		t.Fatalf("NewProductIngredient: %v", err)
	}
	return ing
}

func TestNewProductIngredient_ZeroIngredientID_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewProductIngredient(domain.ID{}, mustQuantity(t, 1000))

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewProductIngredient_ZeroQuantity_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewProductIngredient(domain.NewID(), mustQuantity(t, 0))

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewProduct_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	ingredients := []domain.ProductIngredient{
		mustProductIngredient(t, domain.NewID(), mustQuantity(t, 500)),
	}

	// Act
	product, err := domain.NewProduct(id, tenantID, "X-Burguer", "Hambúrguer artesanal", mustMoney(t, 1500), ingredients)

	// Assert
	if err != nil {
		t.Fatalf("NewProduct returned unexpected error: %v", err)
	}
	if product.ID() != id {
		t.Errorf("product.ID() = %v, want %v", product.ID(), id)
	}
	if product.TenantID() != tenantID {
		t.Errorf("product.TenantID() = %v, want %v", product.TenantID(), tenantID)
	}
	if product.Name() != "X-Burguer" {
		t.Errorf("product.Name() = %q, want %q", product.Name(), "X-Burguer")
	}
	if product.BasePrice().Cents() != 1500 {
		t.Errorf("product.BasePrice().Cents() = %d, want 1500", product.BasePrice().Cents())
	}
	if product.CurrentPrice().Cents() != 1500 {
		t.Errorf("product.CurrentPrice().Cents() = %d, want 1500 (no discount yet)", product.CurrentPrice().Cents())
	}
	if product.Discount() != nil {
		t.Errorf("product.Discount() = %v, want nil", product.Discount())
	}
	if !product.IsActive() {
		t.Error("product.IsActive() = false, want true for a freshly created product")
	}
	if product.Version() != 1 {
		t.Errorf("product.Version() = %d, want 1", product.Version())
	}
	if len(product.Ingredients()) != 1 {
		t.Fatalf("len(product.Ingredients()) = %d, want 1", len(product.Ingredients()))
	}
	if product.CreatedAt().IsZero() {
		t.Error("product.CreatedAt() is zero, want a populated timestamp")
	}
}

func TestNewProduct_EmptyName_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "", "desc", mustMoney(t, 1000), nil)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewProduct_ZeroPrice_ReturnsInvalidError(t *testing.T) {
	// Act
	_, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "Refrigerante", "desc", mustMoney(t, 0), nil)

	// Assert
	if err == nil {
		t.Fatal("NewProduct with zero price returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewProduct_NoIngredients_Succeeds(t *testing.T) {
	// Act: a resold, unprepared product (e.g. a canned drink) legitimately
	// has no tracked ingredient composition.
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "Refrigerante", "Lata 350ml", mustMoney(t, 700), nil)

	// Assert
	if err != nil {
		t.Fatalf("NewProduct with no ingredients returned unexpected error: %v", err)
	}
	if len(product.Ingredients()) != 0 {
		t.Errorf("len(product.Ingredients()) = %d, want 0", len(product.Ingredients()))
	}
}

func TestNewProduct_DuplicateIngredient_ReturnsInvalidError(t *testing.T) {
	// Arrange
	dupID := domain.NewID()
	ingredients := []domain.ProductIngredient{
		mustProductIngredient(t, dupID, mustQuantity(t, 500)),
		mustProductIngredient(t, dupID, mustQuantity(t, 200)),
	}

	// Act
	_, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "X-Burguer", "desc", mustMoney(t, 1500), ingredients)

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestNewProduct_IngredientsAreDefensivelyCopied(t *testing.T) {
	// Arrange
	ingredients := []domain.ProductIngredient{
		mustProductIngredient(t, domain.NewID(), mustQuantity(t, 500)),
	}
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "X-Burguer", "desc", mustMoney(t, 1500), ingredients)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}

	// Act: mutate the caller's original slice after construction.
	ingredients[0] = mustProductIngredient(t, domain.NewID(), mustQuantity(t, 999))

	// Assert: the aggregate's own copy must be unaffected.
	if product.Ingredients()[0].Quantity().Thousandths() != 500 {
		t.Errorf("product.Ingredients() was affected by mutating the caller's original slice")
	}
}

func TestProduct_ApplyDiscount_ValidDiscount_UpdatesPriceAndVersion(t *testing.T) {
	// Arrange
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "X-Burguer", "desc", mustMoney(t, 2000), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	discount := mustDiscountPercent(t, 2500) // 25%

	// Act
	err = product.ApplyDiscount(discount)

	// Assert
	if err != nil {
		t.Fatalf("ApplyDiscount returned unexpected error: %v", err)
	}
	if product.Discount() == nil || product.Discount().Hundredths() != 2500 {
		t.Errorf("product.Discount() = %v, want 2500 hundredths", product.Discount())
	}
	if product.CurrentPrice().Cents() != 1500 {
		t.Errorf("product.CurrentPrice().Cents() = %d, want 1500", product.CurrentPrice().Cents())
	}
	if product.BasePrice().Cents() != 2000 {
		t.Errorf("product.BasePrice().Cents() = %d, want unchanged 2000", product.BasePrice().Cents())
	}
	if product.Version() != 2 {
		t.Errorf("product.Version() = %d, want 2", product.Version())
	}
}

// TestProduct_ApplyDiscount_WouldZeroOutPrice_ReturnsInvalidErrorAndLeavesStateIntact
// proves the aggregate invariant from the task spec: a discount that would
// drive CurrentPrice to zero or below is refused.
func TestProduct_ApplyDiscount_WouldZeroOutPrice_ReturnsInvalidErrorAndLeavesStateIntact(t *testing.T) {
	// Arrange
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "X-Burguer", "desc", mustMoney(t, 2000), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	versionBefore := product.Version()

	// Act: 100% discount would zero out the price.
	err = product.ApplyDiscount(mustDiscountPercent(t, 10000))

	// Assert
	if err == nil {
		t.Fatal("ApplyDiscount(100%) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if product.Discount() != nil {
		t.Errorf("product.Discount() = %v after rejected ApplyDiscount, want nil", product.Discount())
	}
	if product.Version() != versionBefore {
		t.Errorf("product.Version() = %d after rejected ApplyDiscount, want unchanged %d", product.Version(), versionBefore)
	}
	if product.CurrentPrice().Cents() != 2000 {
		t.Errorf("product.CurrentPrice().Cents() = %d after rejected ApplyDiscount, want unchanged 2000", product.CurrentPrice().Cents())
	}
}

// TestProduct_ApplyDiscount_TinyPriceWithSmallDiscount_RoundsToZero_ReturnsInvalidError
// covers the boundary the spec calls out explicitly ("desconto ... não pode
// resultar em preço final negativo/zero"): even a partial discount can
// still round a very small price down to exactly zero cents, and that must
// be rejected exactly like a 100% discount is.
func TestProduct_ApplyDiscount_TinyPriceWithSmallDiscount_RoundsToZero_ReturnsInvalidError(t *testing.T) {
	// Arrange: R$ 0,01 (1 cent) with a 99.99% discount rounds to 0 cents
	// ((1 * 1) + 5000) / 10000 = 0).
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "Adesivo", "desc", mustMoney(t, 1), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}

	// Act
	err = product.ApplyDiscount(mustDiscountPercent(t, 9999))

	// Assert
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestProduct_ApplyDiscount_ReplacesPreviousDiscount(t *testing.T) {
	// Arrange
	product, err := domain.NewProduct(domain.NewID(), mustTenantID(t), "X-Burguer", "desc", mustMoney(t, 2000), nil)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if err := product.ApplyDiscount(mustDiscountPercent(t, 1000)); err != nil {
		t.Fatalf("first ApplyDiscount: %v", err)
	}

	// Act: a second discount replaces the first, it does not compound.
	if err := product.ApplyDiscount(mustDiscountPercent(t, 5000)); err != nil {
		t.Fatalf("second ApplyDiscount: %v", err)
	}

	// Assert
	if product.CurrentPrice().Cents() != 1000 {
		t.Errorf("product.CurrentPrice().Cents() = %d, want 1000 (50%% of 2000, not compounded)", product.CurrentPrice().Cents())
	}
	if product.Version() != 3 {
		t.Errorf("product.Version() = %d, want 3", product.Version())
	}
}
