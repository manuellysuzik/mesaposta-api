package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestLoyaltyEntryType_Valid(t *testing.T) {
	tests := []struct {
		name string
		t    domain.LoyaltyEntryType
		want bool
	}{
		{"credito", domain.LoyaltyEntryTypeCredito, true},
		{"resgate", domain.LoyaltyEntryTypeResgate, true},
		{"empty", domain.LoyaltyEntryType(""), false},
		{"unknown", domain.LoyaltyEntryType("bogus"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.Valid(); got != tt.want {
				t.Errorf("%q.Valid() = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func TestParseLoyaltyEntryType_Valid_Succeeds(t *testing.T) {
	got, err := domain.ParseLoyaltyEntryType("credito")
	if err != nil {
		t.Fatalf("ParseLoyaltyEntryType(\"credito\") returned unexpected error: %v", err)
	}
	if got != domain.LoyaltyEntryTypeCredito {
		t.Errorf("got = %v, want %v", got, domain.LoyaltyEntryTypeCredito)
	}
}

func TestParseLoyaltyEntryType_Unknown_ReturnsInvalidError(t *testing.T) {
	_, err := domain.ParseLoyaltyEntryType("bogus")
	if err == nil {
		t.Fatal("ParseLoyaltyEntryType(\"bogus\") returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}
