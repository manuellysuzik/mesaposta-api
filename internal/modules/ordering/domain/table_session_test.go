package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewTableSession_ZeroTableID_ReturnsInvalid(t *testing.T) {
	_, _, err := domain.NewTableSession(mustOrderTenantID(t), domain.ID{})
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewTableSession_Valid(t *testing.T) {
	tableID := domain.NewID()
	tenantID := mustOrderTenantID(t)

	session, plaintext, err := domain.NewTableSession(tenantID, tableID)
	if err != nil {
		t.Fatalf("NewTableSession: unexpected error: %v", err)
	}
	if plaintext == "" {
		t.Fatal("plaintext token is empty")
	}
	if session.TableID() != tableID {
		t.Errorf("TableID() = %v, want %v", session.TableID(), tableID)
	}
	if session.TenantID() != tenantID {
		t.Errorf("TenantID() = %v, want %v", session.TenantID(), tenantID)
	}
	wantHash := domain.HashSessionToken(plaintext)
	if session.TokenHash() != wantHash {
		t.Error("TokenHash() does not match HashSessionToken(plaintext) -- the plaintext must never be persisted directly")
	}
	if !session.ExpiresAt().After(session.CreatedAt()) {
		t.Error("ExpiresAt() is not after CreatedAt()")
	}
	wantTTL := domain.TableSessionTTL
	gotTTL := session.ExpiresAt().Sub(session.CreatedAt())
	if gotTTL != wantTTL {
		t.Errorf("ExpiresAt()-CreatedAt() = %v, want %v", gotTTL, wantTTL)
	}
}

func TestNewTableSession_ProducesDistinctPlaintextsAndHashes(t *testing.T) {
	tableID := domain.NewID()
	tenantID := mustOrderTenantID(t)

	sessionA, plaintextA, err := domain.NewTableSession(tenantID, tableID)
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}
	sessionB, plaintextB, err := domain.NewTableSession(tenantID, tableID)
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}

	if plaintextA == plaintextB {
		t.Fatal("two calls to NewTableSession produced the same plaintext token")
	}
	if sessionA.TokenHash() == sessionB.TokenHash() {
		t.Fatal("two calls to NewTableSession produced the same token hash")
	}
}

func TestTableSession_IsValid(t *testing.T) {
	session, _, err := domain.NewTableSession(mustOrderTenantID(t), domain.NewID())
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}

	if !session.IsValid(session.CreatedAt()) {
		t.Error("IsValid(createdAt) = false, want true")
	}
	if session.IsValid(session.ExpiresAt().Add(time.Second)) {
		t.Error("IsValid(after expiry) = true, want false")
	}
	if session.IsValid(session.ExpiresAt()) {
		t.Error("IsValid(exactly at expiry) = true, want false (expiry is exclusive)")
	}
}

func TestHashSessionToken_Deterministic(t *testing.T) {
	if domain.HashSessionToken("abc") != domain.HashSessionToken("abc") {
		t.Error("HashSessionToken is not deterministic for the same input")
	}
	if domain.HashSessionToken("abc") == domain.HashSessionToken("abd") {
		t.Error("HashSessionToken produced the same hash for different inputs")
	}
}
