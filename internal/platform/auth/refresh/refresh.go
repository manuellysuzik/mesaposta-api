// Package refresh implements mesaposta-api's rotating opaque refresh
// tokens: high-entropy, random-bytes tokens (never JWTs) that a client
// exchanges for a fresh access token (see internal/platform/auth/token).
//
// Every refresh token belongs to a "family" -- the lineage of tokens
// produced by successive rotations starting from the token minted at
// login. Presenting a valid, not-yet-rotated token via Rotate invalidates
// it and mints a new one in the same family (classic refresh-token
// rotation). Presenting a token that has *already* been rotated once
// before -- i.e. reused -- is treated as a signal that the token was
// stolen and revokes the entire family, not just that one request; see
// Rotate for the detailed reasoning.
//
// Only the SHA-256 hash of a token's plaintext is ever persisted (via
// RefreshStore); the plaintext exists solely in the return value of Issue
// and Rotate, meant to be handed to the client (e.g. via the cookie built
// by NewCookie) and never stored server-side.
package refresh

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const (
	// TokenTTL is how long a freshly issued or rotated refresh token
	// remains valid, and also the MaxAge used by NewCookie.
	TokenTTL = 30 * 24 * time.Hour

	// CookieName is the name of the HTTP cookie carrying the refresh
	// token's plaintext.
	CookieName = "mesaposta_refresh_token"

	// CookiePath scopes the cookie so browsers only ever send it to the
	// auth endpoints that need it, not to every request.
	CookiePath = "/auth"

	// tokenByteLength is the number of cryptographically random bytes
	// used to build a token's plaintext: 32 bytes = 256 bits of entropy,
	// the floor this package commits to.
	tokenByteLength = 32

	// familyIDByteLength is the number of random bytes used to build a
	// FamilyID -- 128 bits is already far more than enough entropy for an
	// identifier that only needs to be unguessable, not itself a secret
	// capable of authenticating anyone.
	familyIDByteLength = 16
)

// Record is a single refresh-token "version" persisted by a RefreshStore.
// Its plaintext token value is never part of Record -- only Hash, the
// SHA-256 digest of that plaintext, is ever persisted.
type Record struct {
	// Hash is the SHA-256 digest of the token's plaintext.
	Hash [32]byte
	// FamilyID groups every token issued or rotated within one continuous
	// login session's lineage. Reuse detection revokes every Record
	// sharing a FamilyID at once.
	FamilyID string
	// SubjectID identifies who this refresh token authenticates.
	//
	// NOTE: intentionally NOT a foreign key to a users table -- that
	// table does not exist yet (it is introduced in Phase 6). For now
	// this is just a loose string/UUID with no referential integrity.
	SubjectID string
	// TenantID is the raw tenant id string this token is scoped to, with
	// the same "not yet a tenant.TenantID" rationale as token.Claims.
	TenantID string
	// IssuedAt is when this specific Record was minted (at login, for the
	// family's first Record, or at rotation time for every subsequent
	// one).
	IssuedAt time.Time
	// ExpiresAt is when this Record stops being acceptable to Rotate,
	// regardless of whether it has been used yet.
	ExpiresAt time.Time
	// RotatedAt is nil while this Record is still its family's current,
	// unused token. It is set the moment Rotate successfully exchanges it
	// for a new Record -- and a Record with RotatedAt already set being
	// presented to Rotate again is exactly the reuse signal that triggers
	// RevokeFamily.
	RotatedAt *time.Time
	// RevokedAt is nil unless this Record's entire family was revoked
	// (reuse detection, explicit logout-everywhere, ...).
	RevokedAt *time.Time
}

// RefreshStore is the persistence port refresh-token logic depends on. The
// concrete Postgres-backed implementation is introduced later
// (infrastructure-layer Bloco E); this package only defines the interface
// and the domain logic that drives it. Tests exercise that logic against
// an in-memory fake implementing this same interface (see refresh_test.go).
type RefreshStore interface {
	// Save persists a brand-new Record -- the first token in a family,
	// minted by Issue.
	Save(ctx context.Context, record Record) error

	// FindByHash returns the Record whose Hash matches hash. It returns a
	// non-nil error (with Kind errs.NotFound, by convention, though
	// callers here never rely on that specific Kind) if no such Record
	// exists.
	FindByHash(ctx context.Context, hash [32]byte) (Record, error)

	// RotateFamily atomically marks the Record identified by oldHash as
	// rotated (RotatedAt = rotatedAt) and persists newRecord as the new
	// current token in the same family. Concrete implementations are
	// expected to perform both writes in a single transaction.
	RotateFamily(ctx context.Context, oldHash [32]byte, rotatedAt time.Time, newRecord Record) error

	// RevokeFamily marks every Record belonging to familyID as revoked
	// (RevokedAt = revokedAt) -- used both for reuse detection and for an
	// explicit "log out everywhere" action.
	RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error
}

// unauthenticated wraps cause as the single public-facing error every
// refresh-token rejection is reported as, regardless of its specific root
// cause (unknown token, expired, already rotated/reused, family revoked).
func unauthenticated(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_refresh_token", "invalid, expired, or revoked refresh token", cause)
}

// newOpaqueToken generates a fresh, cryptographically random plaintext
// token (tokenByteLength bytes, base64url-encoded) and returns it
// alongside the SHA-256 hash that should be the only thing ever persisted
// for it.
func newOpaqueToken() (plaintext string, hash [32]byte, err error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", [32]byte{}, fmt.Errorf("refresh: reading random bytes for token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	hash = sha256.Sum256([]byte(plaintext))
	return plaintext, hash, nil
}

// newFamilyID generates a fresh, random family identifier.
func newFamilyID() (string, error) {
	buf := make([]byte, familyIDByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("refresh: reading random bytes for family id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Issue mints the first refresh token of a brand-new family for subjectID
// within tenantID (typically called once, at successful login), persists
// its Record via store, and returns the plaintext token to hand to the
// client.
func Issue(ctx context.Context, store RefreshStore, subjectID, tenantID string) (string, Record, error) {
	if subjectID == "" {
		return "", Record{}, errs.New(errs.Invalid, "invalid_refresh_subject_id", "subject id must not be empty", nil)
	}
	if tenantID == "" {
		return "", Record{}, errs.New(errs.Invalid, "invalid_refresh_tenant_id", "tenant id must not be empty", nil)
	}

	familyID, err := newFamilyID()
	if err != nil {
		return "", Record{}, err
	}

	plaintext, hash, err := newOpaqueToken()
	if err != nil {
		return "", Record{}, err
	}

	now := time.Now()
	record := Record{
		Hash:      hash,
		FamilyID:  familyID,
		SubjectID: subjectID,
		TenantID:  tenantID,
		IssuedAt:  now,
		ExpiresAt: now.Add(TokenTTL),
	}

	if err := store.Save(ctx, record); err != nil {
		return "", Record{}, fmt.Errorf("refresh: saving new token: %w", err)
	}

	return plaintext, record, nil
}

// Rotate exchanges a presented, still-valid refresh token for a new one in
// the same family, invalidating the presented one in the process.
//
// If the presented token has already been rotated before -- i.e. it is
// being reused -- Rotate treats this as a stolen-token signal (the
// legitimate client already moved on to a newer token; anyone presenting
// this older one again is, by definition, not that client) and revokes
// every Record in the token's family via store.RevokeFamily, denying the
// entire lineage rather than just this one request.
//
// Every failure -- unknown token, expired, already rotated (reused),
// family revoked -- is reported identically as an *errs.Error with Kind
// errs.Unauthenticated, so a caller can never use Rotate's error to probe
// which specific condition applies to a given token.
func Rotate(ctx context.Context, store RefreshStore, presentedPlaintext string) (string, Record, error) {
	if presentedPlaintext == "" {
		return "", Record{}, unauthenticated(errors.New("presented refresh token is empty"))
	}

	presentedHash := sha256.Sum256([]byte(presentedPlaintext))

	record, err := store.FindByHash(ctx, presentedHash)
	if err != nil {
		return "", Record{}, unauthenticated(fmt.Errorf("looking up presented refresh token: %w", err))
	}

	// Defense in depth: verify, in constant time, that the record the
	// store handed back really does match the presented token's hash.
	// The store's own lookup mechanism is expected to already guarantee
	// this, but Rotate never trusts that blindly.
	if !hmac.Equal(record.Hash[:], presentedHash[:]) {
		return "", Record{}, unauthenticated(errors.New("store returned a record whose hash does not match the presented token"))
	}

	now := time.Now()

	if record.RevokedAt != nil {
		return "", Record{}, unauthenticated(fmt.Errorf("refresh token family %q has been revoked", record.FamilyID))
	}

	if record.RotatedAt != nil {
		// Reuse detected. Revoke the whole family, not just this token.
		if revokeErr := store.RevokeFamily(ctx, record.FamilyID, now); revokeErr != nil {
			return "", Record{}, fmt.Errorf("refresh: revoking family %q after reuse detection: %w", record.FamilyID, revokeErr)
		}
		return "", Record{}, unauthenticated(fmt.Errorf("refresh token reuse detected for family %q; family revoked", record.FamilyID))
	}

	if now.After(record.ExpiresAt) {
		return "", Record{}, unauthenticated(fmt.Errorf("refresh token for family %q has expired", record.FamilyID))
	}

	newPlaintext, newHash, err := newOpaqueToken()
	if err != nil {
		return "", Record{}, err
	}

	newRecord := Record{
		Hash:      newHash,
		FamilyID:  record.FamilyID,
		SubjectID: record.SubjectID,
		TenantID:  record.TenantID,
		IssuedAt:  now,
		ExpiresAt: now.Add(TokenTTL),
	}

	if err := store.RotateFamily(ctx, presentedHash, now, newRecord); err != nil {
		return "", Record{}, fmt.Errorf("refresh: rotating family %q: %w", record.FamilyID, err)
	}

	return newPlaintext, newRecord, nil
}

// RevokeFamily revokes every refresh token belonging to familyID (e.g. for
// an explicit "log out everywhere" action).
func RevokeFamily(ctx context.Context, store RefreshStore, familyID string) error {
	if familyID == "" {
		return errs.New(errs.Invalid, "invalid_refresh_family_id", "family id must not be empty", nil)
	}
	if err := store.RevokeFamily(ctx, familyID, time.Now()); err != nil {
		return fmt.Errorf("refresh: revoking family %q: %w", familyID, err)
	}
	return nil
}

// NewCookie builds the http.Cookie used to carry a refresh token's
// plaintext value to the client: HttpOnly and Secure, SameSite=Lax,
// scoped to CookiePath, valid for TokenTTL.
func NewCookie(plaintext string) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    plaintext,
		Path:     CookiePath,
		MaxAge:   int(TokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
