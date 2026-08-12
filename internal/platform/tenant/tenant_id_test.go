package tenant_test

import (
	"encoding"
	"encoding/json"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const validUUID = "11111111-2222-3333-4444-555555555555"

func TestParse_ValidUUID_Succeeds(t *testing.T) {
	// Arrange & Act
	id, err := tenant.Parse(validUUID)

	// Assert
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", validUUID, err)
	}
	if id.IsZero() {
		t.Errorf("Parse(%q) produced a zero-value TenantID", validUUID)
	}
	if id.String() != validUUID {
		t.Errorf("id.String() = %q, want %q", id.String(), validUUID)
	}
}

func TestParse_ValidUppercaseUUID_Succeeds(t *testing.T) {
	// Arrange
	const upper = "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"

	// Act
	id, err := tenant.Parse(upper)

	// Assert
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", upper, err)
	}
	if id.IsZero() {
		t.Errorf("Parse(%q) produced a zero-value TenantID", upper)
	}
}

func TestParse_EmptyString_ReturnsInvalidKindError(t *testing.T) {
	// Arrange & Act
	id, err := tenant.Parse("")

	// Assert
	if err == nil {
		t.Fatal("Parse(\"\") returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Parse(\"\") error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if !id.IsZero() {
		t.Errorf("Parse(\"\") returned non-zero TenantID on error")
	}
}

func TestParse_NotAUUID_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	tests := []string{
		"not-a-uuid",
		"11111111222233334444555555555555",      // missing dashes
		"11111111-2222-3333-4444-55555555555",   // too short
		"11111111-2222-3333-4444-5555555555555", // too long
		"gggggggg-2222-3333-4444-555555555555",  // invalid hex digit
		"11111111_2222_3333_4444_555555555555",  // wrong separator
	}

	for _, in := range tests {
		in := in
		t.Run(in, func(t *testing.T) {
			// Act
			id, err := tenant.Parse(in)

			// Assert
			if err == nil {
				t.Fatalf("Parse(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("Parse(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
			if !id.IsZero() {
				t.Errorf("Parse(%q) returned non-zero TenantID on error", in)
			}
		})
	}
}

func TestParse_NilUUID_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	const nilUUID = "00000000-0000-0000-0000-000000000000"

	// Act
	id, err := tenant.Parse(nilUUID)

	// Assert
	if err == nil {
		t.Fatal("Parse(nil UUID) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("Parse(nil UUID) error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	if !id.IsZero() {
		t.Errorf("Parse(nil UUID) returned non-zero TenantID on error")
	}
}

func TestTenantID_ZeroValue_IsZero(t *testing.T) {
	// Arrange
	var id tenant.TenantID

	// Act & Assert
	if !id.IsZero() {
		t.Errorf("zero-value TenantID.IsZero() = false, want true")
	}
}

func TestTenantID_ZeroValue_StringNeverEmpty(t *testing.T) {
	// Arrange
	var id tenant.TenantID

	// Act
	got := id.String()

	// Assert
	if got == "" {
		t.Fatal("zero-value TenantID.String() returned empty string -- must never be silently empty (log/SQL risk)")
	}
	if got != "<invalid>" {
		t.Errorf("zero-value TenantID.String() = %q, want %q", got, "<invalid>")
	}
}

func TestTenantID_ParsedValue_IsNotZero(t *testing.T) {
	// Arrange & Act
	id, err := tenant.Parse(validUUID)
	if err != nil {
		t.Fatalf("Parse(%q): %v", validUUID, err)
	}

	// Assert
	if id.IsZero() {
		t.Errorf("parsed TenantID reports IsZero() = true, want false")
	}
}

// jsonWrapper is used to prove that TenantID cannot be populated straight
// from an HTTP request body: it must be constructed only via tenant.Parse
// (invoked, in production, from the auth middleware), never via ad-hoc JSON
// deserialization.
type jsonWrapper struct {
	ID tenant.TenantID `json:"id"`
}

func TestTenantID_CannotBeUnmarshaledFromJSON(t *testing.T) {
	// Arrange
	body := []byte(`{"id":"` + validUUID + `"}`)
	var w jsonWrapper

	// Act
	err := json.Unmarshal(body, &w)

	// Assert -- TenantID has no exported fields and no UnmarshalJSON/
	// UnmarshalText method, so json.Unmarshal must fail (type mismatch:
	// JSON string into a struct with no way to accept it) rather than
	// silently populating a forged TenantID.
	if err == nil {
		t.Fatalf("json.Unmarshal succeeded and produced %+v, want an error -- TenantID must not be directly deserializable", w)
	}
}

func TestTenantID_DoesNotImplementUnmarshalInterfaces(t *testing.T) {
	// Arrange
	var id tenant.TenantID
	var asAny any = &id

	// Act & Assert
	if _, ok := asAny.(json.Unmarshaler); ok {
		t.Error("*TenantID must not implement json.Unmarshaler")
	}
	if _, ok := asAny.(encoding.TextUnmarshaler); ok {
		t.Error("*TenantID must not implement encoding.TextUnmarshaler")
	}
}
