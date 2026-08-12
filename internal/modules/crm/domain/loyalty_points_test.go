package domain_test

import (
	"math"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustLoyaltyPoints(t *testing.T, points int64) domain.LoyaltyPoints {
	t.Helper()
	p, err := domain.NewLoyaltyPoints(points)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(%d) returned unexpected error: %v", points, err)
	}
	return p
}

func TestNewLoyaltyPoints_NonNegative_Succeeds(t *testing.T) {
	p, err := domain.NewLoyaltyPoints(100)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(100) returned unexpected error: %v", err)
	}
	if p.Points() != 100 {
		t.Errorf("p.Points() = %d, want 100", p.Points())
	}
}

func TestNewLoyaltyPoints_Zero_Succeeds(t *testing.T) {
	p, err := domain.NewLoyaltyPoints(0)
	if err != nil {
		t.Fatalf("NewLoyaltyPoints(0) returned unexpected error: %v", err)
	}
	if p.Points() != 0 {
		t.Errorf("p.Points() = %d, want 0", p.Points())
	}
}

func TestNewLoyaltyPoints_Negative_ReturnsInvalidError(t *testing.T) {
	_, err := domain.NewLoyaltyPoints(-1)
	if err == nil {
		t.Fatal("NewLoyaltyPoints(-1) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestLoyaltyPoints_Add_SumsCorrectly(t *testing.T) {
	a := mustLoyaltyPoints(t, 100)
	b := mustLoyaltyPoints(t, 50)

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add returned unexpected error: %v", err)
	}
	if sum.Points() != 150 {
		t.Errorf("sum.Points() = %d, want 150", sum.Points())
	}
	// a and b themselves must be untouched (immutability).
	if a.Points() != 100 || b.Points() != 50 {
		t.Errorf("Add mutated its operands: a=%d, b=%d", a.Points(), b.Points())
	}
}

func TestLoyaltyPoints_Add_Overflow_ReturnsInvalidError(t *testing.T) {
	max := mustLoyaltyPoints(t, math.MaxInt64)
	one := mustLoyaltyPoints(t, 1)

	_, err := max.Add(one)
	if err == nil {
		t.Fatal("Add(overflow) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
}

func TestLoyaltyPoints_Sub_LessThanBalance_SucceedsAndSubtractsExactAmount(t *testing.T) {
	balance := mustLoyaltyPoints(t, 100)
	redeem := mustLoyaltyPoints(t, 30)

	remaining, err := balance.Sub(redeem)
	if err != nil {
		t.Fatalf("Sub returned unexpected error: %v", err)
	}
	if remaining.Points() != 70 {
		t.Errorf("remaining.Points() = %d, want 70", remaining.Points())
	}
}

func TestLoyaltyPoints_Sub_ExactBalance_ReachesZero(t *testing.T) {
	balance := mustLoyaltyPoints(t, 100)

	remaining, err := balance.Sub(balance)
	if err != nil {
		t.Fatalf("Sub returned unexpected error: %v", err)
	}
	if remaining.Points() != 0 {
		t.Errorf("remaining.Points() = %d, want 0", remaining.Points())
	}
}

func TestLoyaltyPoints_Sub_MoreThanBalance_ReturnsInvalidErrorWithoutClampingToZero(t *testing.T) {
	balance := mustLoyaltyPoints(t, 10)
	tooMuch := mustLoyaltyPoints(t, 20)

	_, err := balance.Sub(tooMuch)
	if err == nil {
		t.Fatal("Sub(more than balance) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("error kind = %v, want %v", errs.KindOf(err), errs.Invalid)
	}
	// balance itself must be untouched (immutability).
	if balance.Points() != 10 {
		t.Errorf("Sub mutated its receiver: balance.Points() = %d, want unchanged 10", balance.Points())
	}
}
