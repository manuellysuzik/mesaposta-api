package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestRole_Valid(t *testing.T) {
	tests := []struct {
		name string
		role domain.Role
		want bool
	}{
		{"manager", domain.RoleManager, true},
		{"employee", domain.RoleEmployee, true},
		{"empty", domain.Role(""), false},
		{"unknown", domain.Role("admin"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.Valid(); got != tt.want {
				t.Errorf("Role(%q).Valid() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestParseRole(t *testing.T) {
	t.Run("valid values round-trip", func(t *testing.T) {
		for _, want := range []domain.Role{domain.RoleManager, domain.RoleEmployee} {
			got, err := domain.ParseRole(string(want))
			if err != nil {
				t.Fatalf("ParseRole(%q): unexpected error: %v", want, err)
			}
			if got != want {
				t.Errorf("ParseRole(%q) = %q, want %q", want, got, want)
			}
		}
	})

	t.Run("unknown value rejected", func(t *testing.T) {
		_, err := domain.ParseRole("admin")
		if err == nil {
			t.Fatal("ParseRole(\"admin\") = nil error, want an error")
		}
		if !errs.IsKind(err, errs.Invalid) {
			t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
		}
	})

	t.Run("empty value rejected", func(t *testing.T) {
		_, err := domain.ParseRole("")
		if err == nil {
			t.Fatal("ParseRole(\"\") = nil error, want an error")
		}
		if !errs.IsKind(err, errs.Invalid) {
			t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
		}
	})
}
