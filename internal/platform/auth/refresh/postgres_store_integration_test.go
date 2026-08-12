//go:build integration

package refresh_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/refresh"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	_, appURL := db.TestPostgres(t)
	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPostgresStore_SaveAndFindByHash_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantID := "11111111-1111-1111-1111-111111111111"
	plaintext, record, err := refresh.Issue(context.Background(), store, "subject-1", tenantID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if plaintext == "" {
		t.Fatal("Issue: want a non-empty plaintext token")
	}

	tid, err := tenant.Parse(tenantID)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	found, err := store.FindByHash(ctx, record.Hash)
	if err != nil {
		t.Fatalf("FindByHash: %v", err)
	}
	if found.SubjectID != "subject-1" {
		t.Errorf("found.SubjectID = %q, want %q", found.SubjectID, "subject-1")
	}
	if found.FamilyID != record.FamilyID {
		t.Errorf("found.FamilyID = %q, want %q", found.FamilyID, record.FamilyID)
	}
}

func TestPostgresStore_TokenIsNeverStoredInPlaintext(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "22222222-2222-2222-2222-222222222222"
	plaintext, record, err := refresh.Issue(context.Background(), store, "subject-1", tenantIDStr)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tid, err := tenant.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scope.Rollback(ctx)

	var storedHash []byte
	if err := scope.QueryRow(ctx, "SELECT hash FROM refresh_tokens WHERE family_id = $1", record.FamilyID).Scan(&storedHash); err != nil {
		t.Fatalf("reading stored hash column: %v", err)
	}

	if string(storedHash) == plaintext {
		t.Fatal("the stored hash column literally equals the plaintext token -- it must be the SHA-256 digest, not the token itself")
	}
	if strings.Contains(string(storedHash), plaintext) {
		t.Fatal("the stored hash column contains the plaintext token as a substring")
	}
	if len(storedHash) != 32 {
		t.Fatalf("stored hash is %d bytes, want 32 (SHA-256 digest length)", len(storedHash))
	}
}

func TestPostgresStore_ConcurrentRotation_ExactlyOneWinner(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "33333333-3333-3333-3333-333333333333"
	plaintext, _, err := refresh.Issue(context.Background(), store, "subject-1", tenantIDStr)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tid, err := tenant.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	const attempts = 5
	var wg sync.WaitGroup
	successes := make([]bool, attempts)
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			_, _, err := refresh.Rotate(ctx, store, plaintext)
			successes[i] = err == nil
		}(i)
	}
	wg.Wait()

	winners := 0
	for _, ok := range successes {
		if ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful concurrent rotations of the same token, want exactly 1", winners)
	}
}

func TestPostgresStore_ReuseDetection_RevokesEntireFamilyInOneTransaction(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "44444444-4444-4444-4444-444444444444"
	original, _, err := refresh.Issue(context.Background(), store, "subject-1", tenantIDStr)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tid, err := tenant.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	rotated, _, err := refresh.Rotate(ctx, store, original)
	if err != nil {
		t.Fatalf("first Rotate: %v", err)
	}

	// Reuse the already-rotated original token: this must revoke the whole
	// family, including the freshly rotated token.
	if _, _, err := refresh.Rotate(ctx, store, original); err == nil {
		t.Fatal("Rotate on a reused token: want an error, got nil")
	}

	// The freshly rotated (and now family-revoked) token must also be
	// rejected.
	if _, _, err := refresh.Rotate(ctx, store, rotated); err == nil {
		t.Fatal("Rotate on a token from a reuse-revoked family: want an error, got nil")
	}
}

func TestPostgresStore_ExpiredToken_InvisibleToFindByHash(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "55555555-5555-5555-5555-555555555555"
	_, record, err := refresh.Issue(context.Background(), store, "subject-1", tenantIDStr)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tid, err := tenant.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	// Force the record's expires_at into the past directly via SQL (Save
	// always uses TokenTTL, so this bypasses the package's own API on
	// purpose -- it's the only way to construct an expired fixture).
	scope, err := pool.Scope(ctx)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, err := scope.Exec(ctx, "UPDATE refresh_tokens SET expires_at = $1 WHERE family_id = $2",
		time.Now().Add(-time.Hour), record.FamilyID); err != nil {
		t.Fatalf("forcing expiry: %v", err)
	}
	if err := scope.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	_, err = store.FindByHash(ctx, record.Hash)
	if err == nil {
		t.Fatal("FindByHash on an expired token: want an error, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestPostgresStore_Save_InvalidTenantID_ReturnsError(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	record := refresh.Record{
		Hash:      [32]byte{1, 2, 3},
		FamilyID:  "family-x",
		SubjectID: "subject-x",
		TenantID:  "not-a-valid-uuid",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := store.Save(context.Background(), record)
	if err == nil {
		t.Fatal("Save: want an error for a record with a malformed TenantID, got nil")
	}
}

func TestPostgresStore_RotateFamily_InvalidNewRecordTenantID_ReturnsError(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	newRecord := refresh.Record{
		Hash:      [32]byte{9, 9, 9},
		FamilyID:  "family-y",
		SubjectID: "subject-y",
		TenantID:  "also-not-a-valid-uuid",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := store.RotateFamily(context.Background(), [32]byte{1}, time.Now(), newRecord)
	if err == nil {
		t.Fatal("RotateFamily: want an error for a new record with a malformed TenantID, got nil")
	}
}

func TestPostgresStore_RotateFamily_UnknownOldHash_ReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "88888888-8888-8888-8888-888888888888"
	tid, err := tenant.Parse(tenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	ctx := tenant.With(context.Background(), tid)

	newRecord := refresh.Record{
		Hash:      [32]byte{7, 7, 7},
		FamilyID:  "family-z",
		SubjectID: "subject-z",
		TenantID:  tenantIDStr,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	var unknownOldHash [32]byte
	copy(unknownOldHash[:], "this-hash-was-never-saved------")

	err = store.RotateFamily(ctx, unknownOldHash, time.Now(), newRecord)
	if err == nil {
		t.Fatal("RotateFamily: want an error for an oldHash that was never saved, got nil")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestPostgresStore_Save_DuplicateTenantAndHash_ReturnsError(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantIDStr := "99999999-9999-9999-9999-999999999999"
	record := refresh.Record{
		Hash:      [32]byte{4, 4, 4},
		FamilyID:  "family-dup",
		SubjectID: "subject-dup",
		TenantID:  tenantIDStr,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if err := store.Save(context.Background(), record); err != nil {
		t.Fatalf("Save (first): %v", err)
	}

	err := store.Save(context.Background(), record)
	if err == nil {
		t.Fatal("Save (duplicate tenant_id+hash): want an error, got nil")
	}
}

func TestPostgresStore_RLS_TenantIsolation(t *testing.T) {
	pool := openTestPool(t)
	store := refresh.NewPostgresStore(pool)

	tenantAStr := "66666666-6666-6666-6666-666666666666"
	tenantBStr := "77777777-7777-7777-7777-777777777777"

	_, recordA, err := refresh.Issue(context.Background(), store, "subject-a", tenantAStr)
	if err != nil {
		t.Fatalf("Issue (tenant A): %v", err)
	}

	tenantB, err := tenant.Parse(tenantBStr)
	if err != nil {
		t.Fatalf("tenant.Parse (B): %v", err)
	}
	ctxB := tenant.With(context.Background(), tenantB)

	_, err = store.FindByHash(ctxB, recordA.Hash)
	if err == nil {
		t.Fatal("FindByHash: tenant B found tenant A's refresh token, want it hidden by RLS")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}
