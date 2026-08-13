package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// TableSessionTTL is how long a table session token, minted the moment a
// QR-code scan resolves a table via the public cardápio route (see
// NewTableSession), remains valid -- long enough for one dining party to
// browse the menu and place an order, short enough that a leaked/lingering
// token is not useful long after that. 45 minutes was chosen within this
// module's confirmed product-decision window (30-60 minutes).
const TableSessionTTL = 45 * time.Minute

// tableSessionTokenByteLength is the number of cryptographically random
// bytes used to build a session token's plaintext -- 32 bytes = 256 bits of
// entropy, matching internal/platform/auth/refresh's own
// tokenByteLength.
const tableSessionTokenByteLength = 32

// TableSession is the short-lived, opaque credential a QR-code scan mints
// (see NewTableSession), binding an otherwise-unauthenticated diner's
// subsequent POST .../pedidos request to exactly one (tenant, table) pair.
// It is deliberately NOT a JWT and carries no rotation/family lineage --
// this module's confirmed product decision is that this needs to be a much
// simpler "does it exist, is it unexpired, which table does it point at"
// token than internal/platform/auth/refresh's rotating refresh tokens (see
// that package's own doc comment for the fuller rotation scheme this
// module deliberately does NOT replicate).
//
// Only TokenHash is ever persisted (see TableSessionRepository) -- the
// plaintext token exists ONLY in NewTableSession's return value, mirroring
// internal/platform/auth/refresh.Issue's identical "plaintext never
// touches storage" design.
type TableSession struct {
	tenantID  tenant.TenantID
	tableID   ID
	tokenHash [32]byte
	expiresAt time.Time
	createdAt time.Time
}

// HashSessionToken returns the SHA-256 digest of plaintext -- the only
// representation of a session token ever persisted. Exported so the
// application layer can hash a CLIENT-PRESENTED token the exact same way,
// to look an existing TableSession up via
// TableSessionRepository.FindByTokenHash.
func HashSessionToken(plaintext string) [32]byte {
	return sha256.Sum256([]byte(plaintext))
}

// NewTableSession mints a brand-new, random opaque session token bound to
// (tenantID, tableID), valid for TableSessionTTL from now. It returns both
// the TableSession to persist (which carries only the token's hash) and the
// PLAINTEXT token itself -- meant to be handed to the client exactly once,
// in the GET .../cardapio/mesa/{code} response, and never stored
// server-side (see this type's own doc comment).
func NewTableSession(tenantID tenant.TenantID, tableID ID) (session TableSession, plaintext string, err error) {
	if tableID.IsZero() {
		return TableSession{}, "", errs.New(errs.Invalid, "invalid_table_session_table_id",
			"table session table id must not be zero", nil)
	}

	buf := make([]byte, tableSessionTokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return TableSession{}, "", fmt.Errorf("ordering/domain: reading random bytes for table session token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)

	now := time.Now().UTC()
	session = TableSession{
		tenantID:  tenantID,
		tableID:   tableID,
		tokenHash: HashSessionToken(plaintext),
		expiresAt: now.Add(TableSessionTTL),
		createdAt: now,
	}
	return session, plaintext, nil
}

// TenantID returns the tenant this session is scoped to.
func (s TableSession) TenantID() tenant.TenantID { return s.tenantID }

// TableID returns the ID of the Table this session is bound to.
func (s TableSession) TableID() ID { return s.tableID }

// TokenHash returns the SHA-256 hash of the session's plaintext token --
// the only representation ever persisted.
func (s TableSession) TokenHash() [32]byte { return s.tokenHash }

// ExpiresAt returns when this session stops being valid.
func (s TableSession) ExpiresAt() time.Time { return s.expiresAt }

// CreatedAt returns when this session was minted.
func (s TableSession) CreatedAt() time.Time { return s.createdAt }

// IsValid reports whether the session has not yet expired as of now.
func (s TableSession) IsValid(now time.Time) bool {
	return now.Before(s.expiresAt)
}
