package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// uniqueViolationCode is the Postgres error code for a unique-constraint
// violation (SQLSTATE 23505).
const uniqueViolationCode = "23505"

// CustomerRepository is the Postgres-backed implementation of
// domain.CustomerRepository, backed by the customers table (see
// internal/platform/db/migrations/0006_crm.sql). Every method runs against
// an already-open db.Scope -- there is deliberately no "tenant_id = $n"
// predicate in any SELECT/UPDATE below, since Postgres row-level security
// (the tenant_isolation policy on customers) already restricts every
// statement to the scope's own tenant; adding a redundant application-level
// predicate would only invite the two to silently drift apart over time.
// Every rehydrated Customer's TenantID is set from scope.TenantID() --
// never re-parsed from the row's own tenant_id column (which architecture
// rule R6 would forbid this package from doing via tenant.Parse anyway) --
// since RLS already guarantees that column can only ever equal the scope's
// tenant.
type CustomerRepository struct {
	scope *db.Scope
}

// compile-time check: *CustomerRepository must satisfy domain.CustomerRepository.
var _ domain.CustomerRepository = (*CustomerRepository)(nil)

// NewCustomerRepository builds a CustomerRepository bound to scope. Most
// callers reach a CustomerRepository via UnitOfWork.Do/Repositories.
// Customers() instead; this constructor exists so this package's own
// integration test suite (and any other caller that already holds a
// *db.Scope) can exercise it directly, in isolation from the rest of
// Repositories.
func NewCustomerRepository(scope *db.Scope) *CustomerRepository {
	return &CustomerRepository{scope: scope}
}

// customerNotFoundErr builds the *errs.Error a missing-row lookup returns.
func customerNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "customer_not_found", "customer not found", cause)
}

// customerConflictErr builds the *errs.Error UpdateWithVersion returns when
// its UPDATE affects zero rows -- see domain.CustomerRepository's doc
// comment for the full optimistic-concurrency contract this implements.
func customerConflictErr() error {
	return errs.New(errs.Conflict, "customer_version_conflict",
		"customer was concurrently modified", nil)
}

// customerDuplicateContactErr builds the *errs.Error Insert returns when
// the database's per-tenant unique constraint on email or phone (see
// 0006_crm.sql) rejects the row.
func customerDuplicateContactErr(cause error) error {
	return errs.New(errs.Conflict, "customer_duplicate_contact",
		"a customer with this email or phone already exists for this tenant", cause)
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

const selectCustomerColumns = `id, name, email, phone, loyalty_points, version, created_at, updated_at`

// customerRow is satisfied by pgx.Row, the shape FindByID's single-row
// query returns.
type customerRow interface {
	Scan(dest ...any) error
}

// scanCustomerRow scans a single customers row (in the exact column order
// selectCustomerColumns selects) and rehydrates it into a *domain.Customer
// via domain.NewCustomer plus the fields that constructor cannot know
// about (LoyaltyPoints, Version, CreatedAt, UpdatedAt) -- see hydrate.go's
// doc comment for why that requires setUnexportedField.
func scanCustomerRow(row customerRow, tenantID tenant.TenantID) (*domain.Customer, error) {
	var (
		idStr     string
		name      string
		email     *string
		phone     *string
		pointsRaw int64
		version   int64
		createdAt time.Time
		updatedAt time.Time
	)

	if err := row.Scan(&idStr, &name, &email, &phone, &pointsRaw, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("crm/postgres: parsing customer id: %w", err)
	}

	customer, err := domain.NewCustomer(id, tenantID, name, email, phone)
	if err != nil {
		return nil, fmt.Errorf("crm/postgres: rehydrating customer: %w", err)
	}

	points, err := domain.NewLoyaltyPoints(pointsRaw)
	if err != nil {
		return nil, fmt.Errorf("crm/postgres: building customer loyalty points: %w", err)
	}

	setUnexportedField(&customer, "loyaltyPoints", points)
	setUnexportedField(&customer, "version", version)
	setUnexportedField(&customer, "createdAt", createdAt.UTC())
	setUnexportedField(&customer, "updatedAt", updatedAt.UTC())

	return &customer, nil
}

// FindByID returns the Customer identified by id, or a NotFound *errs.Error
// if none exists (or none is visible to the scope's tenant, per RLS).
func (r *CustomerRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Customer, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectCustomerColumns+" FROM customers WHERE id = $1", id.String())

	customer, err := scanCustomerRow(row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, customerNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("crm/postgres: finding customer by id: %w", err)
	}
	return customer, nil
}

// Insert persists a brand-new Customer. A per-tenant unique-constraint
// violation on email or phone (see 0006_crm.sql) is reported as an
// *errs.Error of Kind errs.Conflict, not a raw database error.
func (r *CustomerRepository) Insert(ctx context.Context, customer *domain.Customer) error {
	var emailArg, phoneArg any
	if email := customer.Email(); email != nil {
		emailArg = *email
	}
	if phone := customer.Phone(); phone != nil {
		phoneArg = *phone
	}

	_, err := r.scope.Exec(ctx, `
		INSERT INTO customers (id, tenant_id, name, email, phone, loyalty_points, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		customer.ID().String(), r.scope.TenantID().String(), customer.Name(), emailArg, phoneArg,
		customer.LoyaltyPoints().Points(), customer.Version(), customer.CreatedAt(), customer.UpdatedAt(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return customerDuplicateContactErr(err)
		}
		return fmt.Errorf("crm/postgres: inserting customer: %w", err)
	}
	return nil
}

// UpdateWithVersion persists customer's current loyalty point balance,
// enforcing optimistic concurrency against expectedVersion: "WHERE id = $1
// AND version = $2". Only loyalty_points, version and updated_at are
// written -- name, email and phone never change through
// Customer.CreditPoints/RedeemPoints (the only mutations that ever call
// UpdateWithVersion in this phase; there is no "update customer profile"
// use case yet -- see this module's final report for that deliberately
// deferred scope decision), so there is nothing else for this statement to
// persist.
func (r *CustomerRepository) UpdateWithVersion(ctx context.Context, customer *domain.Customer, expectedVersion int64) error {
	tag, err := r.scope.Exec(ctx, `
		UPDATE customers
		SET loyalty_points = $1, version = version + 1, updated_at = $2
		WHERE id = $3 AND version = $4
	`, customer.LoyaltyPoints().Points(), customer.UpdatedAt(), customer.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("crm/postgres: updating customer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return customerConflictErr()
	}
	return nil
}
