package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// TableSessionRepository is the Postgres-backed implementation of
// domain.TableSessionRepository, backed by the (append-only)
// table_sessions table (see
// internal/platform/db/migrations/0009_ordering.sql). There is no update:
// a TableSession is immutable once minted -- see domain/table_session.go's
// own doc comment.
type TableSessionRepository struct {
	scope *db.Scope
}

// compile-time check: *TableSessionRepository must satisfy
// domain.TableSessionRepository.
var _ domain.TableSessionRepository = (*TableSessionRepository)(nil)

// NewTableSessionRepository builds a TableSessionRepository bound to
// scope.
func NewTableSessionRepository(scope *db.Scope) *TableSessionRepository {
	return &TableSessionRepository{scope: scope}
}

func tableSessionNotFoundErr(cause error) error {
	return errs.New(errs.NotFound, "table_session_not_found", "table session not found", cause)
}

// Insert persists a brand-new TableSession.
func (r *TableSessionRepository) Insert(ctx context.Context, session domain.TableSession) error {
	tokenHash := session.TokenHash()
	if _, err := r.scope.Exec(ctx, `
		INSERT INTO table_sessions (tenant_id, table_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`,
		r.scope.TenantID().String(), session.TableID().String(), tokenHash[:],
		session.ExpiresAt(), session.CreatedAt(),
	); err != nil {
		return fmt.Errorf("ordering/postgres: inserting table session: %w", err)
	}
	return nil
}

// FindByTokenHash returns the TableSession whose TokenHash matches hash,
// or a NotFound *errs.Error if none exists (or none is visible to the
// scope's tenant, per RLS). Does NOT filter out an expired session's row
// itself -- see domain.TableSessionRepository.FindByTokenHash's own doc
// comment for why that is safe: callers always re-check
// TableSession.IsValid on the result.
func (r *TableSessionRepository) FindByTokenHash(ctx context.Context, hash [32]byte) (*domain.TableSession, error) {
	row := r.scope.QueryRow(ctx, `
		SELECT table_id, token_hash, expires_at, created_at
		FROM table_sessions
		WHERE token_hash = $1
	`, hash[:])

	var (
		tableIDStr string
		hashBytes  []byte
		expiresAt  time.Time
		createdAt  time.Time
	)
	err := row.Scan(&tableIDStr, &hashBytes, &expiresAt, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tableSessionNotFoundErr(err)
	}
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: finding table session by token hash: %w", err)
	}

	tableID, err := domain.ParseID(tableIDStr)
	if err != nil {
		return nil, fmt.Errorf("ordering/postgres: parsing table session table id: %w", err)
	}

	// domain.TableSession has no public constructor that accepts an
	// arbitrary, already-persisted hash/timestamps (mirroring every other
	// aggregate's own "NewX always starts fresh" shape -- see
	// domain/table_session.go's own doc comment: NewTableSession always
	// mints a brand-new random token). Rehydration here goes through
	// reflection, exactly like every other repository's hydrate-from-row
	// path in this package.
	session := domain.TableSession{}
	setUnexportedField(&session, "tenantID", r.scope.TenantID())
	setUnexportedField(&session, "tableID", tableID)
	var hashArray [32]byte
	copy(hashArray[:], hashBytes)
	setUnexportedField(&session, "tokenHash", hashArray)
	setUnexportedField(&session, "expiresAt", expiresAt.UTC())
	setUnexportedField(&session, "createdAt", createdAt.UTC())

	return &session, nil
}
