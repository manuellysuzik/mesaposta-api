package refresh_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/refresh"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// memStore is an in-memory fake implementing refresh.RefreshStore, used to
// exercise the rotation/reuse-detection domain logic without a real
// database. It also exposes a few test-only helpers (insert, get) to set
// up fixtures directly.
type memStore struct {
	mu      sync.Mutex
	records map[[32]byte]refresh.Record
}

func newMemStore() *memStore {
	return &memStore{records: map[[32]byte]refresh.Record{}}
}

func (s *memStore) Save(_ context.Context, record refresh.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.Hash] = record
	return nil
}

func (s *memStore) FindByHash(_ context.Context, hash [32]byte) (refresh.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[hash]
	if !ok {
		return refresh.Record{}, errs.New(errs.NotFound, "refresh_token_not_found", "refresh token not found", nil)
	}
	return record, nil
}

func (s *memStore) RotateFamily(_ context.Context, oldHash [32]byte, rotatedAt time.Time, newRecord refresh.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.records[oldHash]
	if !ok {
		return errs.New(errs.NotFound, "refresh_token_not_found", "refresh token not found", nil)
	}
	rotatedAtCopy := rotatedAt
	old.RotatedAt = &rotatedAtCopy
	s.records[oldHash] = old
	s.records[newRecord.Hash] = newRecord
	return nil
}

func (s *memStore) RevokeFamily(_ context.Context, familyID string, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, record := range s.records {
		if record.FamilyID != familyID {
			continue
		}
		revokedAtCopy := revokedAt
		record.RevokedAt = &revokedAtCopy
		s.records[hash] = record
	}
	return nil
}

func (s *memStore) get(hash [32]byte) (refresh.Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[hash]
	return r, ok
}

func (s *memStore) insert(record refresh.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.Hash] = record
}

func hashOf(plaintext string) [32]byte {
	return sha256.Sum256([]byte(plaintext))
}

func TestIssue_ReturnsPlaintextWhoseHashMatchesTheStoredRecord(t *testing.T) {
	store := newMemStore()

	plaintext, record, err := refresh.Issue(context.Background(), store, "subject-1", "tenant-1")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	if plaintext == "" {
		t.Fatal("Issue() returned an empty plaintext token")
	}
	if want := hashOf(plaintext); record.Hash != want {
		t.Errorf("record.Hash = %x, want sha256(plaintext) = %x", record.Hash, want)
	}
	if record.FamilyID == "" {
		t.Error("record.FamilyID is empty, want a generated family id")
	}
	if record.SubjectID != "subject-1" {
		t.Errorf("record.SubjectID = %q, want %q", record.SubjectID, "subject-1")
	}
	if record.TenantID != "tenant-1" {
		t.Errorf("record.TenantID = %q, want %q", record.TenantID, "tenant-1")
	}
	if record.RotatedAt != nil {
		t.Error("record.RotatedAt is set on a freshly issued token, want nil")
	}
	if record.RevokedAt != nil {
		t.Error("record.RevokedAt is set on a freshly issued token, want nil")
	}

	wantExpiry := record.IssuedAt.Add(refresh.TokenTTL)
	if !record.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("record.ExpiresAt = %v, want %v (IssuedAt + TokenTTL)", record.ExpiresAt, wantExpiry)
	}

	stored, ok := store.get(record.Hash)
	if !ok {
		t.Fatal("Issue() did not persist the record via store.Save")
	}
	if stored.FamilyID != record.FamilyID {
		t.Errorf("stored.FamilyID = %q, want %q", stored.FamilyID, record.FamilyID)
	}
}

func TestIssue_TokenHasAtLeast256BitsOfEntropy(t *testing.T) {
	store := newMemStore()

	plaintext, _, err := refresh.Issue(context.Background(), store, "subject-1", "tenant-1")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	// base64.RawURLEncoding of 32 random bytes (256 bits) always produces
	// exactly 43 characters (ceil(32*8/6)). Anything shorter could not
	// possibly encode 256 bits of entropy.
	const minPlaintextLen = 43
	if len(plaintext) < minPlaintextLen {
		t.Errorf("len(plaintext) = %d, want >= %d (256 bits of entropy, base64url-encoded)", len(plaintext), minPlaintextLen)
	}
}

func TestIssue_GeneratesUniqueTokensAndFamiliesAcrossManyCalls(t *testing.T) {
	store := newMemStore()

	const n = 100
	seenTokens := make(map[string]bool, n)
	seenFamilies := make(map[string]bool, n)

	for i := 0; i < n; i++ {
		plaintext, record, err := refresh.Issue(context.Background(), store, "subject-1", "tenant-1")
		if err != nil {
			t.Fatalf("Issue() call #%d returned error: %v", i, err)
		}
		if seenTokens[plaintext] {
			t.Fatalf("Issue() call #%d produced a plaintext token seen before: %q", i, plaintext)
		}
		if seenFamilies[record.FamilyID] {
			t.Fatalf("Issue() call #%d produced a family id seen before: %q", i, record.FamilyID)
		}
		seenTokens[plaintext] = true
		seenFamilies[record.FamilyID] = true
	}
}

func TestIssue_RejectsEmptySubjectOrTenantID(t *testing.T) {
	store := newMemStore()

	tests := []struct {
		name      string
		subjectID string
		tenantID  string
	}{
		{"empty subject id", "", "tenant-1"},
		{"empty tenant id", "subject-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := refresh.Issue(context.Background(), store, tt.subjectID, tt.tenantID); err == nil {
				t.Fatal("Issue(): expected error, got nil")
			}
		})
	}
}

func TestRotate_ValidTokenRotatesToANewTokenInTheSameFamily(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	plaintext1, record1, err := refresh.Issue(ctx, store, "subject-1", "tenant-1")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	plaintext2, record2, err := refresh.Rotate(ctx, store, plaintext1)
	if err != nil {
		t.Fatalf("Rotate() returned error: %v", err)
	}

	if plaintext2 == plaintext1 {
		t.Fatal("Rotate() returned the same plaintext as the presented token")
	}
	if record2.FamilyID != record1.FamilyID {
		t.Errorf("record2.FamilyID = %q, want %q (same family)", record2.FamilyID, record1.FamilyID)
	}
	if record2.SubjectID != record1.SubjectID {
		t.Errorf("record2.SubjectID = %q, want %q", record2.SubjectID, record1.SubjectID)
	}
	if record2.RotatedAt != nil {
		t.Error("record2.RotatedAt is set on a freshly rotated token, want nil")
	}

	oldStored, ok := store.get(record1.Hash)
	if !ok {
		t.Fatal("original record no longer present in store after rotation")
	}
	if oldStored.RotatedAt == nil {
		t.Error("original record's RotatedAt is nil after rotation, want it set")
	}
}

func TestRotate_ReusedTokenRevokesTheEntireFamily(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	plaintext1, record1, err := refresh.Issue(ctx, store, "subject-1", "tenant-1")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	plaintext2, _, err := refresh.Rotate(ctx, store, plaintext1)
	if err != nil {
		t.Fatalf("first Rotate() returned error: %v", err)
	}

	// Replay the already-rotated first token -- this must fail AND revoke
	// the whole family.
	_, _, err = refresh.Rotate(ctx, store, plaintext1)
	if err == nil {
		t.Fatal("Rotate() with a reused (already-rotated) token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}

	// The legitimate, still-current token (plaintext2) must now ALSO be
	// rejected, proving the entire family was revoked -- not just the
	// reused token.
	_, _, err = refresh.Rotate(ctx, store, plaintext2)
	if err == nil {
		t.Fatal("Rotate() with the family's current token after reuse detection: expected error (family revoked), got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}

	revoked, ok := store.get(record1.Hash)
	if !ok {
		t.Fatal("original record missing from store")
	}
	if revoked.RevokedAt == nil {
		t.Error("original record's RevokedAt is nil after reuse detection, want it set")
	}
}

func TestRotate_ExpiredTokenIsUnauthenticated(t *testing.T) {
	store := newMemStore()

	plaintext := "expired-token-fixture-value-0123456789"
	hash := hashOf(plaintext)
	store.insert(refresh.Record{
		Hash:      hash,
		FamilyID:  "family-1",
		SubjectID: "subject-1",
		TenantID:  "tenant-1",
		IssuedAt:  time.Now().Add(-2 * refresh.TokenTTL),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	_, _, err := refresh.Rotate(context.Background(), store, plaintext)
	if err == nil {
		t.Fatal("Rotate() with an expired token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestRotate_UnknownTokenIsUnauthenticated(t *testing.T) {
	store := newMemStore()

	_, _, err := refresh.Rotate(context.Background(), store, "this-token-was-never-issued")
	if err == nil {
		t.Fatal("Rotate() with an unknown token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestRotate_EmptyTokenIsUnauthenticated(t *testing.T) {
	store := newMemStore()

	_, _, err := refresh.Rotate(context.Background(), store, "")
	if err == nil {
		t.Fatal("Rotate() with an empty token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestRotate_AlreadyRevokedFamilyIsUnauthenticated(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	plaintext, record, err := refresh.Issue(ctx, store, "subject-1", "tenant-1")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	if err := refresh.RevokeFamily(ctx, store, record.FamilyID); err != nil {
		t.Fatalf("RevokeFamily() returned error: %v", err)
	}

	_, _, err = refresh.Rotate(ctx, store, plaintext)
	if err == nil {
		t.Fatal("Rotate() on a revoked family: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestRevokeFamily_RejectsEmptyFamilyID(t *testing.T) {
	store := newMemStore()

	if err := refresh.RevokeFamily(context.Background(), store, ""); err == nil {
		t.Fatal("RevokeFamily(): expected error, got nil")
	}
}

func TestNewCookie_HasExpectedSecurityAttributes(t *testing.T) {
	cookie := refresh.NewCookie("some-plaintext-token")

	if cookie.Name != refresh.CookieName {
		t.Errorf("Name = %q, want %q", cookie.Name, refresh.CookieName)
	}
	if cookie.Value != "some-plaintext-token" {
		t.Errorf("Value = %q, want %q", cookie.Value, "some-plaintext-token")
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly = false, want true")
	}
	if !cookie.Secure {
		t.Error("Secure = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want %v (Lax)", cookie.SameSite, http.SameSiteLaxMode)
	}
	if cookie.Path != "/auth" {
		t.Errorf("Path = %q, want %q", cookie.Path, "/auth")
	}

	wantMaxAge := int((30 * 24 * time.Hour).Seconds())
	if cookie.MaxAge != wantMaxAge {
		t.Errorf("MaxAge = %d, want %d (30 days)", cookie.MaxAge, wantMaxAge)
	}
}
