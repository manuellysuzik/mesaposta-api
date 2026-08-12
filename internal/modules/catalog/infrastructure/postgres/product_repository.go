package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// ProductRepository is the Postgres-backed implementation of
// domain.ProductRepository, backed by the products and product_ingredients
// tables (see internal/platform/db/migrations/0005_catalog.sql). Every
// method runs against an already-open db.Scope -- there is deliberately no
// "tenant_id = $n" predicate in any SELECT/UPDATE below, since Postgres
// row-level security (the tenant_isolation policy on both tables) already
// restricts every statement to the scope's own tenant; adding a redundant
// application-level predicate would only invite the two to silently drift
// apart over time. Every rehydrated Product's TenantID is set from
// scope.TenantID() -- never re-parsed from the row's own tenant_id column
// (which architecture rule R6 would forbid this package from doing via
// tenant.Parse anyway) -- since RLS already guarantees that column can only
// ever equal the scope's tenant.
type ProductRepository struct {
	scope *db.Scope
}

// compile-time check: *ProductRepository must satisfy domain.ProductRepository.
var _ domain.ProductRepository = (*ProductRepository)(nil)

// NewProductRepository builds a ProductRepository bound to scope. Most
// callers reach a ProductRepository via UnitOfWork.Do/Repositories.
// Products() instead; this constructor exists so this package's own
// integration test suite (and any other caller that already holds a
// *db.Scope) can exercise it directly, in isolation from the rest of
// Repositories.
func NewProductRepository(scope *db.Scope) *ProductRepository {
	return &ProductRepository{scope: scope}
}

// productNotFoundErr builds the *errs.Error a missing-row lookup returns.
func productNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "product_not_found", "product not found", cause)
}

// productConflictErr builds the *errs.Error UpdateWithVersion returns when
// its UPDATE affects zero rows -- see domain.ProductRepository's doc
// comment for the full optimistic-concurrency contract this implements.
func productConflictErr() error {
	return errs.New(errs.Conflict, "product_version_conflict",
		"product was concurrently modified", nil)
}

const selectProductColumns = `
	id, name, description, (base_price * 100)::bigint, (discount_percent * 100)::bigint,
	is_active, version, created_at, updated_at
`

const selectProductIngredientsSQL = `
	SELECT ingredient_id, (quantity * 1000)::bigint
	FROM product_ingredients
	WHERE product_id = $1
	ORDER BY ingredient_id ASC
`

// productRow is satisfied by pgx.Row, the shape both FindByID's and
// FindActive's row-scanning helpers need.
type productRow interface {
	Scan(dest ...any) error
}

// scanProductRow scans a single products row (in the exact column order
// selectProductColumns selects), loads its ingredient lines via a second
// query against the SAME scope (and therefore the same Postgres
// transaction), and rehydrates the whole thing into a *domain.Product via
// domain.NewProduct plus the fields that constructor cannot know about
// (Discount, IsActive, Version, CreatedAt, UpdatedAt) -- see hydrate.go's
// doc comment for why that requires setUnexportedField.
func (r *ProductRepository) scanProductRow(ctx context.Context, row productRow, tenantID tenant.TenantID) (*domain.Product, error) {
	var (
		idStr        string
		name         string
		description  string
		basePriceRaw int64
		discountRaw  *int64
		isActive     bool
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
	)

	if err := row.Scan(&idStr, &name, &description, &basePriceRaw, &discountRaw, &isActive, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: parsing product id: %w", err)
	}
	basePrice, err := domain.NewMoney(basePriceRaw)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: building product base price: %w", err)
	}

	ingredients, err := r.loadProductIngredients(ctx, idStr)
	if err != nil {
		return nil, err
	}

	product, err := domain.NewProduct(id, tenantID, name, description, basePrice, ingredients)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: rehydrating product: %w", err)
	}

	if discountRaw != nil {
		discount, err := domain.NewDiscountPercent(*discountRaw)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: building product discount: %w", err)
		}
		setUnexportedField(&product, "discount", &discount)
	}
	setUnexportedField(&product, "isActive", isActive)
	setUnexportedField(&product, "version", version)
	setUnexportedField(&product, "createdAt", createdAt.UTC())
	setUnexportedField(&product, "updatedAt", updatedAt.UTC())

	return &product, nil
}

// loadProductIngredients loads every product_ingredients row for
// productIDStr, rehydrating each into a domain.ProductIngredient.
func (r *ProductRepository) loadProductIngredients(ctx context.Context, productIDStr string) ([]domain.ProductIngredient, error) {
	rows, err := r.scope.Query(ctx, selectProductIngredientsSQL, productIDStr)
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: querying product ingredients: %w", err)
	}
	defer rows.Close()

	var ingredients []domain.ProductIngredient
	for rows.Next() {
		var (
			ingredientIDStr string
			quantityRaw     int64
		)
		if err := rows.Scan(&ingredientIDStr, &quantityRaw); err != nil {
			return nil, fmt.Errorf("catalog/postgres: scanning product ingredient row: %w", err)
		}

		ingredientID, err := domain.ParseID(ingredientIDStr)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: parsing product ingredient id: %w", err)
		}
		quantity, err := domain.NewQuantity(quantityRaw)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: building product ingredient quantity: %w", err)
		}

		ingredient, err := domain.NewProductIngredient(ingredientID, quantity)
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: rehydrating product ingredient: %w", err)
		}
		ingredients = append(ingredients, ingredient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog/postgres: iterating product ingredient rows: %w", err)
	}

	return ingredients, nil
}

// FindByID returns the Product identified by id, or a NotFound
// *errs.Error if none exists (or none is visible to the scope's tenant,
// per RLS).
func (r *ProductRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Product, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectProductColumns+" FROM products WHERE id = $1", id.String())

	product, err := r.scanProductRow(ctx, row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, productNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: finding product by id: %w", err)
	}
	return product, nil
}

// FindActive returns every active Product visible to the scope's tenant,
// ordered ascending by name.
func (r *ProductRepository) FindActive(ctx context.Context) ([]*domain.Product, error) {
	rows, err := r.scope.Query(ctx,
		"SELECT "+selectProductColumns+" FROM products WHERE is_active = true ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("catalog/postgres: querying active products: %w", err)
	}
	defer rows.Close()

	var out []*domain.Product
	for rows.Next() {
		product, err := r.scanProductRow(ctx, rows, r.scope.TenantID())
		if err != nil {
			return nil, fmt.Errorf("catalog/postgres: scanning product row: %w", err)
		}
		out = append(out, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog/postgres: iterating product rows: %w", err)
	}

	return out, nil
}

// Insert persists a brand-new Product, along with every one of its
// ingredient lines.
func (r *ProductRepository) Insert(ctx context.Context, product *domain.Product) error {
	var discountArg any
	if d := product.Discount(); d != nil {
		discountArg = d.Hundredths()
	}

	if _, err := r.scope.Exec(ctx, `
		INSERT INTO products (id, tenant_id, name, description, base_price, discount_percent, is_active, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::numeric / 100, $6::numeric / 100, $7, $8, $9, $10)
	`,
		product.ID().String(), r.scope.TenantID().String(), product.Name(), product.Description(),
		product.BasePrice().Cents(), discountArg, product.IsActive(), product.Version(),
		product.CreatedAt(), product.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("catalog/postgres: inserting product: %w", err)
	}

	for _, ingredient := range product.Ingredients() {
		if _, err := r.scope.Exec(ctx, `
			INSERT INTO product_ingredients (tenant_id, product_id, ingredient_id, quantity)
			VALUES ($1, $2, $3, $4::numeric / 1000)
		`,
			r.scope.TenantID().String(), product.ID().String(), ingredient.IngredientID().String(),
			ingredient.Quantity().Thousandths(),
		); err != nil {
			return fmt.Errorf("catalog/postgres: inserting product ingredient: %w", err)
		}
	}

	return nil
}

// UpdateWithVersion persists product's current discount_percent/is_active,
// enforcing optimistic concurrency against expectedVersion: "WHERE id = $1
// AND version = $2". Only discount_percent, is_active, version and
// updated_at are written -- name, description, base_price and ingredients
// never change through Product.ApplyDiscount (this phase's only mutation
// that ever calls UpdateWithVersion), so there is nothing else for this
// statement to persist.
func (r *ProductRepository) UpdateWithVersion(ctx context.Context, product *domain.Product, expectedVersion int64) error {
	var discountArg any
	if d := product.Discount(); d != nil {
		discountArg = d.Hundredths()
	}

	tag, err := r.scope.Exec(ctx, `
		UPDATE products
		SET discount_percent = $1::numeric / 100, is_active = $2, version = version + 1, updated_at = $3
		WHERE id = $4 AND version = $5
	`, discountArg, product.IsActive(), product.UpdatedAt(), product.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("catalog/postgres: updating product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return productConflictErr()
	}
	return nil
}
