package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func mustUserTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("tenant.Parse: %v", err)
	}
	return id
}

func TestNewUser_Valid(t *testing.T) {
	tenantID := mustUserTenantID(t)
	id := domain.NewID()

	before := time.Now().UTC()
	user, err := domain.NewUser(id, tenantID, "owner@example.com", "some-hash", domain.RoleGestor)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewUser: unexpected error: %v", err)
	}

	if user.ID() != id {
		t.Errorf("ID() = %v, want %v", user.ID(), id)
	}
	if user.TenantID() != tenantID {
		t.Errorf("TenantID() = %v, want %v", user.TenantID(), tenantID)
	}
	if user.Email() != "owner@example.com" {
		t.Errorf("Email() = %q, want %q", user.Email(), "owner@example.com")
	}
	if user.PasswordHash() != "some-hash" {
		t.Errorf("PasswordHash() = %q, want %q", user.PasswordHash(), "some-hash")
	}
	if user.Role() != domain.RoleGestor {
		t.Errorf("Role() = %q, want %q", user.Role(), domain.RoleGestor)
	}
	if !user.IsActive() {
		t.Error("IsActive() = false, want true for a freshly constructed user")
	}
	if user.CreatedAt().Before(before) || user.CreatedAt().After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", user.CreatedAt(), before, after)
	}
	if !user.UpdatedAt().Equal(user.CreatedAt()) {
		t.Errorf("UpdatedAt() = %v, want equal to CreatedAt() = %v", user.UpdatedAt(), user.CreatedAt())
	}
}

func TestNewUser_InvalidEmail(t *testing.T) {
	tenantID := mustUserTenantID(t)

	tests := []struct {
		name  string
		email string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"leading whitespace", " owner@example.com"},
		{"trailing whitespace", "owner@example.com "},
		{"internal whitespace", "owner @example.com"},
		{"no at sign", "owner.example.com"},
		{"at sign at start", "@example.com"},
		{"at sign at end", "owner@"},
		{"two at signs", "owner@ex@ample.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewUser(domain.NewID(), tenantID, tt.email, "some-hash", domain.RoleGestor)
			if err == nil {
				t.Fatalf("NewUser(email=%q) = nil error, want an error", tt.email)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
			}
		})
	}
}

func TestNewUser_InvalidPasswordHash(t *testing.T) {
	tenantID := mustUserTenantID(t)

	_, err := domain.NewUser(domain.NewID(), tenantID, "owner@example.com", "  ", domain.RoleGestor)
	if err == nil {
		t.Fatal("NewUser(passwordHash=blank) = nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewUser_InvalidRole(t *testing.T) {
	tenantID := mustUserTenantID(t)

	_, err := domain.NewUser(domain.NewID(), tenantID, "owner@example.com", "some-hash", domain.Role("admin"))
	if err == nil {
		t.Fatal("NewUser(role=\"admin\") = nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
