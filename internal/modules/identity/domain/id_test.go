package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewID_ProducesDistinctNonZeroIDs(t *testing.T) {
	a := domain.NewID()
	b := domain.NewID()

	if a.IsZero() {
		t.Fatal("NewID() returned a zero-value ID")
	}
	if a == b {
		t.Fatal("two calls to NewID() returned the same ID")
	}
}

func TestNewID_StringRoundTripsThroughParseID(t *testing.T) {
	id := domain.NewID()

	parsed, err := domain.ParseID(id.String())
	if err != nil {
		t.Fatalf("ParseID(%q): unexpected error: %v", id.String(), err)
	}
	if parsed != id {
		t.Fatalf("ParseID(id.String()) = %v, want %v", parsed, id)
	}
}

func TestParseID_Invalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"too short", "11111111-1111-1111-1111-11111111111"},
		{"too long", "11111111-1111-1111-1111-1111111111111"},
		{"missing dash", "111111111111-1111-1111-111111111111"},
		{"non-hex digit", "gggggggg-1111-1111-1111-111111111111"},
		{"nil uuid", "00000000-0000-0000-0000-000000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ParseID(tt.in)
			if err == nil {
				t.Fatalf("ParseID(%q) = nil error, want an error", tt.in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
			}
		})
	}
}

func TestID_IsZero_ZeroValue(t *testing.T) {
	var id domain.ID
	if !id.IsZero() {
		t.Error("zero-value ID.IsZero() = false, want true")
	}
	if id.String() != "<invalid>" {
		t.Errorf("zero-value ID.String() = %q, want %q", id.String(), "<invalid>")
	}
}
