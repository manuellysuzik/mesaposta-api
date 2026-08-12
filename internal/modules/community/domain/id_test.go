package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewID_ProducesDistinctNonZeroIDs(t *testing.T) {
	a := domain.NewID()
	b := domain.NewID()

	if a.IsZero() {
		t.Fatal("NewID(): got zero-value ID")
	}
	if a == b {
		t.Fatal("NewID(): two calls produced the same ID")
	}
}

func TestParseID_ValidUUID_RoundTrips(t *testing.T) {
	original := domain.NewID()

	parsed, err := domain.ParseID(original.String())
	if err != nil {
		t.Fatalf("ParseID(%q): unexpected error: %v", original.String(), err)
	}
	if parsed != original {
		t.Errorf("ParseID round-trip = %v, want %v", parsed, original)
	}
}

func TestParseID_Invalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"nil uuid", "00000000-0000-0000-0000-000000000000"},
		{"wrong length", "not-a-uuid"},
		{"missing dashes", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"non-hex digit", "gggggggg-gggg-gggg-gggg-gggggggggggg"},
		{"dash in wrong position", "aaaaaaaa-aaaa-aaaa-aaaaa-aaaaaaaaaaa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ParseID(tt.in)
			if err == nil {
				t.Fatalf("ParseID(%q): want error, got nil", tt.in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
			}
		})
	}
}

func TestID_IsZero_ZeroValue(t *testing.T) {
	var zero domain.ID
	if !zero.IsZero() {
		t.Error("zero-value ID.IsZero() = false, want true")
	}
	if domain.NewID().IsZero() {
		t.Error("NewID().IsZero() = true, want false")
	}
}

func TestID_String_ZeroValue_RendersInvalidMarker(t *testing.T) {
	var zero domain.ID
	if got := zero.String(); got != "<invalid>" {
		t.Errorf("zero.String() = %q, want %q", got, "<invalid>")
	}
}
