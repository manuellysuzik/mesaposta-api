package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewMoney_Negative_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewMoney(-1)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewMoney_Zero_IsValid(t *testing.T) {
	m, err := domain.NewMoney(0)
	if err != nil {
		t.Fatalf("NewMoney(0): unexpected error: %v", err)
	}
	if m.Cents() != 0 {
		t.Errorf("Cents() = %d, want 0", m.Cents())
	}
}

func TestMoney_Add(t *testing.T) {
	a, _ := domain.NewMoney(100)
	b, _ := domain.NewMoney(250)

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if sum.Cents() != 350 {
		t.Errorf("sum.Cents() = %d, want 350", sum.Cents())
	}
	// a and b must remain untouched (immutability).
	if a.Cents() != 100 || b.Cents() != 250 {
		t.Errorf("Add mutated its operands: a=%d, b=%d", a.Cents(), b.Cents())
	}
}

func TestMoney_Add_Overflow(t *testing.T) {
	a, _ := domain.NewMoney(math.MaxInt64)
	b, _ := domain.NewMoney(1)

	_, err := a.Add(b)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestMoney_MulInt(t *testing.T) {
	tests := []struct {
		name      string
		cents     int64
		count     int64
		wantCents int64
		wantErr   bool
	}{
		{"zero count", 500, 0, 0, false},
		{"typical", 1500, 3, 4500, false},
		{"one", 999, 1, 999, false},
		{"negative count", 500, -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := domain.NewMoney(tt.cents)
			if err != nil {
				t.Fatalf("NewMoney: %v", err)
			}
			got, err := m.MulInt(tt.count)
			if tt.wantErr {
				if !errs.IsKind(err, errs.Invalid) {
					t.Fatalf("MulInt: errs.KindOf = %v, want errs.Invalid", errs.KindOf(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("MulInt: unexpected error: %v", err)
			}
			if got.Cents() != tt.wantCents {
				t.Errorf("MulInt(%d) = %d, want %d", tt.count, got.Cents(), tt.wantCents)
			}
		})
	}
}

func TestMoney_MulInt_Overflow(t *testing.T) {
	m, _ := domain.NewMoney(math.MaxInt64)
	_, err := m.MulInt(2)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
