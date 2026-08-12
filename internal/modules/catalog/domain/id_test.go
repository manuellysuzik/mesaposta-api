package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const validID = "11111111-2222-3333-4444-555555555555"

func TestParseID_ValidUUID_Succeeds(t *testing.T) {
	// Arrange & Act
	id, err := domain.ParseID(validID)

	// Assert
	if err != nil {
		t.Fatalf("ParseID(%q) returned unexpected error: %v", validID, err)
	}
	if id.IsZero() {
		t.Errorf("ParseID(%q) produced a zero-value ID", validID)
	}
	if id.String() != validID {
		t.Errorf("id.String() = %q, want %q", id.String(), validID)
	}
}

func TestParseID_Invalid_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"not a uuid at all", "not-a-uuid"},
		{"missing dashes", "11111111222233334444555555555555"},
		{"too short", "11111111-2222-3333-4444-55555555555"},
		{"too long", "11111111-2222-3333-4444-5555555555555"},
		{"invalid hex digit", "gggggggg-2222-3333-4444-555555555555"},
		{"nil uuid", "00000000-0000-0000-0000-000000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			id, err := domain.ParseID(tt.in)

			// Assert
			if err == nil {
				t.Fatalf("ParseID(%q) returned nil error, want an error", tt.in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseID(%q) error kind = %v, want %v", tt.in, errs.KindOf(err), errs.Invalid)
			}
			if !id.IsZero() {
				t.Errorf("ParseID(%q) returned non-zero ID on error", tt.in)
			}
		})
	}
}

func TestID_ZeroValue_IsZero(t *testing.T) {
	// Arrange
	var id domain.ID

	// Act & Assert
	if !id.IsZero() {
		t.Errorf("zero-value ID.IsZero() = false, want true")
	}
	if id.String() != "<invalid>" {
		t.Errorf("zero-value ID.String() = %q, want %q", id.String(), "<invalid>")
	}
}

func TestNewID_ProducesDistinctReparsableValues(t *testing.T) {
	// Arrange
	const n = 200
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		// Act
		id := domain.NewID()

		// Assert
		if id.IsZero() {
			t.Fatalf("NewID() #%d produced a zero-value ID", i)
		}
		text := id.String()
		if _, dup := seen[text]; dup {
			t.Fatalf("NewID() #%d produced duplicate value %q", i, text)
		}
		seen[text] = struct{}{}

		reparsed, err := domain.ParseID(text)
		if err != nil {
			t.Fatalf("ParseID(NewID().String()) #%d returned unexpected error: %v", i, err)
		}
		if reparsed.String() != text {
			t.Fatalf("ParseID(NewID().String()) #%d round-trip mismatch: got %q, want %q", i, reparsed.String(), text)
		}
	}
}
