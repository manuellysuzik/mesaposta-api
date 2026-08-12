package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
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

func TestParseID_ValidUppercaseUUID_Succeeds(t *testing.T) {
	// Arrange
	const upper = "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"

	// Act
	id, err := domain.ParseID(upper)

	// Assert
	if err != nil {
		t.Fatalf("ParseID(%q) returned unexpected error: %v", upper, err)
	}
	if id.IsZero() {
		t.Errorf("ParseID(%q) produced a zero-value ID", upper)
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
		{"wrong separator", "11111111_2222_3333_4444_555555555555"},
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
}

func TestID_ZeroValue_StringNeverEmpty(t *testing.T) {
	// Arrange
	var id domain.ID

	// Act
	got := id.String()

	// Assert
	if got == "" {
		t.Fatal("zero-value ID.String() returned empty string -- must never be silently empty (log/SQL risk)")
	}
	if got != "<invalid>" {
		t.Errorf("zero-value ID.String() = %q, want %q", got, "<invalid>")
	}
}

func TestNewID_ProducesDistinctReparsableValues(t *testing.T) {
	// Arrange
	const n = 1000
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		// Act
		id := domain.NewID()

		// Assert: not zero.
		if id.IsZero() {
			t.Fatalf("NewID() #%d produced a zero-value ID", i)
		}

		text := id.String()

		// Assert: distinct from every previously generated value.
		if _, dup := seen[text]; dup {
			t.Fatalf("NewID() #%d produced duplicate value %q", i, text)
		}
		seen[text] = struct{}{}

		// Assert: re-parseable without error.
		reparsed, err := domain.ParseID(text)
		if err != nil {
			t.Fatalf("ParseID(NewID().String()) #%d returned unexpected error: %v", i, err)
		}
		if reparsed.String() != text {
			t.Fatalf("ParseID(NewID().String()) #%d round-trip mismatch: got %q, want %q", i, reparsed.String(), text)
		}
	}
}

func TestNewID_HasVersion4AndRFC4122Variant(t *testing.T) {
	// Arrange & Act
	id := domain.NewID()
	text := id.String()

	// Assert: version nibble (first char of the 3rd group) is '4'.
	if text[14] != '4' {
		t.Errorf("NewID().String() = %q, version nibble at index 14 = %q, want %q", text, string(text[14]), "4")
	}

	// Assert: variant nibble (first char of the 4th group) is one of 8,9,a,b
	// (the RFC 4122 variant, binary 10xxxxxx).
	variantNibble := text[19]
	switch variantNibble {
	case '8', '9', 'a', 'b', 'A', 'B':
		// ok
	default:
		t.Errorf("NewID().String() = %q, variant nibble at index 19 = %q, want one of [89abAB]", text, string(variantNibble))
	}
}
