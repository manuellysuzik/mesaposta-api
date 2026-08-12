package refresh

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// PostgresStore implements RefreshStore against the refresh_tokens table
// (see internal/platform/db/migrations/0003_refresh_tokens.sql), scoping
// every statement through db.Scope -- and therefore through Postgres
// row-level security.
//
// Every RefreshStore method requires ctx to carry the token's tenant (via
// internal/platform/tenant.With), because db.Scope has no other way to
// know which tenant a query is scoped to. For Save and RotateFamily that
// tenant is available directly on the Record being written (its TenantID
// field), so those two methods parse it themselves and attach it to ctx --
// callers do not need to pre-populate ctx for them.
//
// FindByHash and RevokeFamily, however, are NOT given a tenant by the
// RefreshStore interface itself (by design: refresh.Rotate looks a
// presented token up by its opaque hash alone, before it knows which
// tenant issued it). Callers of THIS adapter's FindByHash/RevokeFamily
// must therefore already have placed the correct tenant.TenantID into ctx
// before calling them -- typically derived from the request's host/path
// (a tenant identifier a multi-tenant deployment can establish before
// looking at the refresh cookie's contents at all), never from the opaque
// token itself. This is a deliberate consequence of this codebase's core
// invariant that no Postgres query ever runs without an authenticated
// tenant in hand (see internal/platform/db's package doc comment): a
// cross-tenant "search every tenant's tokens for this hash" query is not
// something this adapter will ever perform.
type PostgresStore struct {
	pool *db.Pool
}

// compile-time check: *PostgresStore must satisfy RefreshStore.
var _ RefreshStore = (*PostgresStore)(nil)

// NewPostgresStore builds a PostgresStore backed by pool.
func NewPostgresStore(pool *db.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const insertRecordSQL = `
INSERT INTO refresh_tokens (tenant_id, hash, family_id, subject_id, issued_at, expires_at, rotated_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

// Save persists record, deriving its tenant scope from record.TenantID
// (not from ctx, which may not carry any tenant yet at Issue time -- Save
// is typically called right after a fresh login, before any access token
// exists).
func (s *PostgresStore) Save(ctx context.Context, record Record) error {
	tenantID, err := tenant.Parse(record.TenantID)
	if err != nil {
		return fmt.Errorf("refresh/postgres: parsing record tenant id: %w", err)
	}
	ctx = tenant.With(ctx, tenantID)

	scope, err := s.pool.Scope(ctx)
	if err != nil {
		return fmt.Errorf("refresh/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	if _, err := scope.Exec(ctx, insertRecordSQL,
		tenantID.String(), record.Hash[:], record.FamilyID, record.SubjectID,
		record.IssuedAt, record.ExpiresAt, record.RotatedAt, record.RevokedAt,
	); err != nil {
		return fmt.Errorf("refresh/postgres: saving record: %w", err)
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("refresh/postgres: committing save: %w", err)
	}
	return nil
}

// notFoundErr is the single, uniform error FindByHash returns for "no
// record" -- whether because the hash genuinely does not exist, or because
// it belongs to an expired token this method deliberately hides (see the
// method's own doc comment).
func notFoundErr(cause error) error {
	return errs.New(errs.NotFound, "refresh_token_not_found", "refresh token not found", cause)
}

// FindByHash looks up the Record whose Hash matches hash, WITHIN the
// tenant already attached to ctx (see PostgresStore's package/type doc
// comment for why ctx must already carry it). Expired tokens are
// deliberately invisible to FindByHash -- an expired token presented to
// Rotate ends up reported as "not found" here rather than reaching
// refresh.Rotate's own expiry check, but since every failure mode Rotate
// reports collapses to the same errs.Unauthenticated regardless, this is
// externally indistinguishable.
func (s *PostgresStore) FindByHash(ctx context.Context, hash [32]byte) (Record, error) {
	scope, err := s.pool.Scope(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("refresh/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	record, err := scanRecord(scope.QueryRow(ctx, `
		SELECT tenant_id, hash, family_id, subject_id, issued_at, expires_at, rotated_at, revoked_at
		FROM refresh_tokens
		WHERE hash = $1 AND expires_at > now()
	`, hash[:]))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, notFoundErr(err)
	}
	if err != nil {
		return Record{}, fmt.Errorf("refresh/postgres: finding record by hash: %w", err)
	}

	if err := scope.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("refresh/postgres: committing lookup: %w", err)
	}
	return record, nil
}

// alreadyRotatedOrRevokedErr signals RotateFamily's own conflict-detection
// path -- distinct from Rotate's (refresh.go) reuse-detection, which
// operates on FindByHash's result instead. This is the guard that makes
// two concurrent RotateFamily calls presenting the SAME oldHash
// deterministically produce exactly one winner: see RotateFamily's doc
// comment.
func alreadyRotatedOrRevokedErr() error {
	return errs.New(errs.Conflict, "refresh_token_already_rotated",
		"refresh token has already been rotated or revoked", nil)
}

// RotateFamily atomically marks oldHash's Record as rotated and inserts
// newRecord, deriving its tenant scope from newRecord.TenantID.
//
// Two goroutines racing to rotate the SAME oldHash (e.g. a legitimate
// client retrying a dropped response, or -- the scenario this genuinely
// guards against -- a stolen token being replayed concurrently with the
// legitimate client's own request) must produce exactly one winner. This
// is achieved with `SELECT ... FOR UPDATE`: the row identified by oldHash
// is locked for the duration of the transaction, so the second transaction
// blocks until the first commits, then re-reads rotated_at/revoked_at
// (now non-NULL, since the first transaction already committed its
// update) and fails with alreadyRotatedOrRevokedErr instead of rotating
// the same token twice.
func (s *PostgresStore) RotateFamily(ctx context.Context, oldHash [32]byte, rotatedAt time.Time, newRecord Record) error {
	tenantID, err := tenant.Parse(newRecord.TenantID)
	if err != nil {
		return fmt.Errorf("refresh/postgres: parsing new record tenant id: %w", err)
	}
	ctx = tenant.With(ctx, tenantID)

	scope, err := s.pool.Scope(ctx)
	if err != nil {
		return fmt.Errorf("refresh/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	var existingRotatedAt, existingRevokedAt *time.Time
	err = scope.QueryRow(ctx, `
		SELECT rotated_at, revoked_at
		FROM refresh_tokens
		WHERE tenant_id = $1 AND hash = $2
		FOR UPDATE
	`, tenantID.String(), oldHash[:]).Scan(&existingRotatedAt, &existingRevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFoundErr(err)
	}
	if err != nil {
		return fmt.Errorf("refresh/postgres: locking old record: %w", err)
	}
	if existingRotatedAt != nil || existingRevokedAt != nil {
		return alreadyRotatedOrRevokedErr()
	}

	if _, err := scope.Exec(ctx, `
		UPDATE refresh_tokens SET rotated_at = $1
		WHERE tenant_id = $2 AND hash = $3
	`, rotatedAt, tenantID.String(), oldHash[:]); err != nil {
		return fmt.Errorf("refresh/postgres: marking old record rotated: %w", err)
	}

	if _, err := scope.Exec(ctx, insertRecordSQL,
		tenantID.String(), newRecord.Hash[:], newRecord.FamilyID, newRecord.SubjectID,
		newRecord.IssuedAt, newRecord.ExpiresAt, newRecord.RotatedAt, newRecord.RevokedAt,
	); err != nil {
		return fmt.Errorf("refresh/postgres: inserting rotated record: %w", err)
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("refresh/postgres: committing rotation: %w", err)
	}
	return nil
}

// RevokeFamily revokes every Record belonging to familyID, WITHIN the
// tenant already attached to ctx (see PostgresStore's package/type doc
// comment), as a single UPDATE inside a single transaction -- reuse
// detection and an explicit "log out everywhere" action both rely on this
// being all-or-nothing across the whole family.
func (s *PostgresStore) RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error {
	scope, err := s.pool.Scope(ctx)
	if err != nil {
		return fmt.Errorf("refresh/postgres: opening tenant scope: %w", err)
	}
	defer func() { _ = scope.Rollback(ctx) }()

	if _, err := scope.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = $1
		WHERE family_id = $2 AND revoked_at IS NULL
	`, revokedAt, familyID); err != nil {
		return fmt.Errorf("refresh/postgres: revoking family: %w", err)
	}

	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("refresh/postgres: committing revocation: %w", err)
	}
	return nil
}

// rowScanner is satisfied by pgx.Row, factored out purely so scanRecord
// can be unit tested (see postgres_store_test.go) against a hand-rolled
// fake instead of a real pgx.Row.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanRecord scans a single refresh_tokens row (in the exact column order
// FindByHash and RotateFamily's lookup query select) into a Record.
func scanRecord(row rowScanner) (Record, error) {
	var record Record
	var tenantIDStr string
	var hashBytes []byte

	if err := row.Scan(
		&tenantIDStr, &hashBytes, &record.FamilyID, &record.SubjectID,
		&record.IssuedAt, &record.ExpiresAt, &record.RotatedAt, &record.RevokedAt,
	); err != nil {
		return Record{}, err
	}

	record.TenantID = tenantIDStr
	copy(record.Hash[:], hashBytes)
	return record, nil
}
