package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewID_ProducesNonZeroDistinctIDs(t *testing.T) {
	a := domain.NewID()
	b := domain.NewID()

	if a.IsZero() {
		t.Fatal("NewID: IsZero() = true, want false")
	}
	if a.String() == b.String() {
		t.Fatal("NewID: two calls produced the same ID")
	}
}

func TestParseID_RoundTrip(t *testing.T) {
	id := domain.NewID()

	got, err := domain.ParseID(id.String())
	if err != nil {
		t.Fatalf("ParseID(%q): unexpected error: %v", id.String(), err)
	}
	if got.String() != id.String() {
		t.Errorf("ParseID round trip = %q, want %q", got.String(), id.String())
	}
}

func TestParseID_Invalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"too short", "aaaaaaaa"},
		{"missing dashes", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"non-hex digit", "gggggggg-gggg-gggg-gggg-gggggggggggg"},
		{"nil UUID", "00000000-0000-0000-0000-000000000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ParseID(tt.in)
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseID(%q): errs.KindOf = %v, want errs.Invalid", tt.in, errs.KindOf(err))
			}
		})
	}
}

func TestID_String_ZeroValue(t *testing.T) {
	var zero domain.ID
	if got := zero.String(); got != "<invalid>" {
		t.Errorf("zero ID.String() = %q, want %q", got, "<invalid>")
	}
}
