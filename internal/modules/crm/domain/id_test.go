package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewID_GeneratesNonZeroID(t *testing.T) {
	id := domain.NewID()
	if id.IsZero() {
		t.Error("NewID() returned a zero ID")
	}
}

func TestNewID_GeneratesDistinctIDs(t *testing.T) {
	a := domain.NewID()
	b := domain.NewID()
	if a == b {
		t.Error("two calls to NewID() returned the same ID")
	}
}

func TestParseID_ValidUUID_RoundTrips(t *testing.T) {
	original := domain.NewID()
	parsed, err := domain.ParseID(original.String())
	if err != nil {
		t.Fatalf("ParseID(%q) returned unexpected error: %v", original.String(), err)
	}
	if parsed != original {
		t.Errorf("ParseID round-trip = %v, want %v", parsed, original)
	}
}

func TestParseID_Empty_ReturnsInvalidError(t *testing.T) {
	_, err := domain.ParseID("")
	if err == nil {
		t.Fatal("ParseID(\"\") returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestParseID_NilUUID_ReturnsInvalidError(t *testing.T) {
	_, err := domain.ParseID("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("ParseID(nil UUID) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestParseID_MalformedUUID_ReturnsInvalidError(t *testing.T) {
	_, err := domain.ParseID("not-a-uuid")
	if err == nil {
		t.Fatal("ParseID(malformed) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestID_String_ZeroValue_RendersInvalid(t *testing.T) {
	var zero domain.ID
	if got := zero.String(); got != "<invalid>" {
		t.Errorf("zero ID.String() = %q, want %q", got, "<invalid>")
	}
}
