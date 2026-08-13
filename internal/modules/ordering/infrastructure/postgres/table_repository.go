package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// uniqueViolationCode is the Postgres error code for a unique-constraint
// violation (SQLSTATE 23505).
const uniqueViolationCode = "23505"

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

// TableRepository is the Postgres-backed implementation of
// domain.TableRepository, backed by the tables table (see
// internal/platform/db/migrations/0009_ordering.sql). Every method runs
// against an already-open db.Scope -- there is deliberately no
// "tenant_id = $n" predicate in any SELECT/UPDATE below, since Postgres
// row-level security (the tenant_isolation policy on tables) already
// restricts every statement to the scope's own tenant.
type TableRepository struct {
	scope *db.Scope
}

// compile-time check: *TableRepository must satisfy domain.TableRepository.
var _ domain.TableRepository = (*TableRepository)(nil)

// NewTableRepository builds a TableRepository bound to scope. Most callers
// reach a TableRepository via UnitOfWork.Do/Repositories.Tables() instead;
// this constructor exists so this package's own integration test suite
// (and any other caller that already holds a *db.Scope) can exercise it
// directly.
func NewTableRepository(scope *db.Scope) *TableRepository {
	return &TableRepository{scope: scope}
}

func tableNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "table_not_found", "table not found", cause)
}

func tableConflictErr() error {
	return errs.New(errs.Conflict, "table_version_conflict", "table was concurrently modified", nil)
}

const selectTableColumns = `id, code, label, is_active, version, created_at, updated_at`

type tableRow interface {
	Scan(dest ...any) error
}

func scanTableRow(row tableRow, tenantID tenant.TenantID) (*domain.Table, error) {
	var (
		idStr     string
		code      string
		label     string
		isActive  bool
		version   int64
		createdAt time.Time
		updatedAt time.Time
	)

	if err := row.Scan(&idStr, &code, &label, &isActive, &version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	id, err := domain.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: parsing table id: %w", err)
	}

	table, err := domain.NewTable(id, tenantID, code, label)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: rehydrating table: %w", err)
	}

	setUnexportedField(&table, "isActive", isActive)
	setUnexportedField(&table, "version", version)
	setUnexportedField(&table, "createdAt", createdAt.UTC())
	setUnexportedField(&table, "updatedAt", updatedAt.UTC())

	return &table, nil
}

// FindByID returns the Table identified by id, or a NotFound *errs.Error
// if none exists (or none is visible to the scope's tenant, per RLS).
func (r *TableRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Table, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectTableColumns+" FROM tables WHERE id = $1", id.String())

	table, err := scanTableRow(row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tableNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: finding table by id: %w", err)
	}
	return table, nil
}

// FindByCode returns the Table identified by code, or a NotFound
// *errs.Error if none exists -- regardless of whether it is active; see
// domain.TableRepository's own doc comment for why that collapsing is the
// CALLER's (../../application.ResolveTableForMenu's) responsibility, not
// this query's.
func (r *TableRepository) FindByCode(ctx context.Context, code string) (*domain.Table, error) {
	row := r.scope.QueryRow(ctx, "SELECT "+selectTableColumns+" FROM tables WHERE code = $1", code)

	table, err := scanTableRow(row, r.scope.TenantID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tableNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: finding table by code: %w", err)
	}
	return table, nil
}

// FindAll returns every Table visible to the scope's tenant, ordered
// ascending by code.
func (r *TableRepository) FindAll(ctx context.Context) ([]*domain.Table, error) {
	rows, err := r.scope.Query(ctx, "SELECT "+selectTableColumns+" FROM tables ORDER BY code ASC")
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: querying tables: %w", err)
	}
	defer rows.Close()

	var out []*domain.Table
	for rows.Next() {
		table, err := scanTableRow(rows, r.scope.TenantID())
		if err != nil {
			return nil, fmt.Errorf("ordering/postgres: scanning table row: %w", err)
		}
		out = append(out, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ordering/postgres: iterating table rows: %w", err)
	}
	return out, nil
}

// Insert persists a brand-new Table. A per-tenant unique-constraint
// violation on code (see 0009_ordering.sql) is reported as an
// *errs.Error of Kind errs.Conflict, not a raw database error.
func (r *TableRepository) Insert(ctx context.Context, table *domain.Table) error {
	_, err := r.scope.Exec(ctx, `
		INSERT INTO tables (id, tenant_id, code, label, is_active, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		table.ID().String(), r.scope.TenantID().String(), table.Code(), table.Label(),
		table.IsActive(), table.Version(), table.CreatedAt(), table.UpdatedAt(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return errs.New(errs.Conflict, "table_duplicate_code",
				"a table with this code already exists for this tenant", err)
		}
		return fmt.Errorf("ordering/postgres: inserting table: %w", err)
	}
	return nil
}

// UpdateWithVersion persists table's current is_active/updated_at,
// enforcing optimistic concurrency against expectedVersion.
func (r *TableRepository) UpdateWithVersion(ctx context.Context, table *domain.Table, expectedVersion int64) error {
	tag, err := r.scope.Exec(ctx, `
		UPDATE tables
		SET is_active = $1, version = version + 1, updated_at = $2
		WHERE id = $3 AND version = $4
	`, table.IsActive(), table.UpdatedAt(), table.ID().String(), expectedVersion)
	if err != nil {
		return fmt.Errorf("ordering/postgres: updating table: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tableConflictErr()
	}
	return nil
}
