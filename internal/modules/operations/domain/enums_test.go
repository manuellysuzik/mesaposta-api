package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// TestStockUnit_ValidValues_RoundTripAndAreExhaustive pins down the
// complete, closed set of valid StockUnit values. If someone adds a new
// unit to the Valid()/Parse switch without updating this list (or vice
// versa), this test fails -- which is the point: nobody should be able to
// widen this enum without also remembering to update the database CHECK
// constraint added in a later phase.
func TestStockUnit_ValidValues_RoundTripAndAreExhaustive(t *testing.T) {
	// Arrange
	want := []domain.StockUnit{
		domain.StockUnitKg,
		domain.StockUnitG,
		domain.StockUnitL,
		domain.StockUnitMl,
		domain.StockUnitUn,
	}
	wantStrings := []string{"kg", "g", "l", "ml", "un"}

	for i, v := range want {
		v, wantString := v, wantStrings[i]
		t.Run(wantString, func(t *testing.T) {
			// Assert: literal value matches the expected wire string.
			if string(v) != wantString {
				t.Errorf("value #%d = %q, want %q", i, string(v), wantString)
			}
			// Assert: Valid() agrees this is a known value.
			if !v.Valid() {
				t.Errorf("StockUnit(%q).Valid() = false, want true", v)
			}
			// Act & Assert: Parse round-trips.
			parsed, err := domain.ParseStockUnit(string(v))
			if err != nil {
				t.Fatalf("ParseStockUnit(%q) returned unexpected error: %v", v, err)
			}
			if parsed != v {
				t.Errorf("ParseStockUnit(%q) = %q, want %q", v, parsed, v)
			}
		})
	}

	assertExhaustiveStockUnit(t, want)
}

// assertExhaustiveStockUnit fails if any StockUnit value outside want
// reports Valid() == true, catching a new enum member added without
// updating this test's want list.
func assertExhaustiveStockUnit(t *testing.T, want []domain.StockUnit) {
	t.Helper()
	knownCandidates := []domain.StockUnit{
		domain.StockUnitKg, domain.StockUnitG, domain.StockUnitL, domain.StockUnitMl, domain.StockUnitUn,
		domain.StockUnit("unknown_candidate_not_in_enum"),
	}
	wantSet := make(map[domain.StockUnit]bool, len(want))
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range knownCandidates {
		if v.Valid() != wantSet[v] {
			t.Errorf("StockUnit(%q).Valid() = %v, want %v (exhaustiveness check)", v, v.Valid(), wantSet[v])
		}
	}
}

func TestStockUnit_ZeroValue_IsInvalid(t *testing.T) {
	// Arrange
	var u domain.StockUnit

	// Act & Assert
	if u.Valid() {
		t.Error("zero-value StockUnit.Valid() = true, want false")
	}
}

func TestParseStockUnit_Unknown_ReturnsInvalidKindError(t *testing.T) {
	// Arrange
	tests := []string{"", "kilograms", "KG", "unidade"}

	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			// Act
			_, err := domain.ParseStockUnit(in)

			// Assert
			if err == nil {
				t.Fatalf("ParseStockUnit(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseStockUnit(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}

// TestStockMovementType_ValidValues_RoundTripAndAreExhaustive mirrors
// TestStockUnit_ValidValues_RoundTripAndAreExhaustive for
// StockMovementType.
func TestStockMovementType_ValidValues_RoundTripAndAreExhaustive(t *testing.T) {
	// Arrange
	want := []domain.StockMovementType{
		domain.StockMovementTypeEntrada,
		domain.StockMovementTypeSaida,
	}
	wantStrings := []string{"entrada", "saida"}

	for i, v := range want {
		v, wantString := v, wantStrings[i]
		t.Run(wantString, func(t *testing.T) {
			if string(v) != wantString {
				t.Errorf("value #%d = %q, want %q", i, string(v), wantString)
			}
			if !v.Valid() {
				t.Errorf("StockMovementType(%q).Valid() = false, want true", v)
			}
			parsed, err := domain.ParseStockMovementType(string(v))
			if err != nil {
				t.Fatalf("ParseStockMovementType(%q) returned unexpected error: %v", v, err)
			}
			if parsed != v {
				t.Errorf("ParseStockMovementType(%q) = %q, want %q", v, parsed, v)
			}
		})
	}

	wantSet := map[domain.StockMovementType]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range []domain.StockMovementType{
		domain.StockMovementTypeEntrada, domain.StockMovementTypeSaida,
		domain.StockMovementType("unknown_candidate_not_in_enum"),
	} {
		if v.Valid() != wantSet[v] {
			t.Errorf("StockMovementType(%q).Valid() = %v, want %v (exhaustiveness check)", v, v.Valid(), wantSet[v])
		}
	}
}

func TestStockMovementType_ZeroValue_IsInvalid(t *testing.T) {
	var v domain.StockMovementType
	if v.Valid() {
		t.Error("zero-value StockMovementType.Valid() = true, want false")
	}
}

func TestParseStockMovementType_Unknown_ReturnsInvalidKindError(t *testing.T) {
	tests := []string{"", "in", "out", "ENTRADA"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := domain.ParseStockMovementType(in)
			if err == nil {
				t.Fatalf("ParseStockMovementType(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseStockMovementType(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}

// TestPurchaseOrderStatus_ValidValues_RoundTripAndAreExhaustive mirrors the
// pattern above for PurchaseOrderStatus.
func TestPurchaseOrderStatus_ValidValues_RoundTripAndAreExhaustive(t *testing.T) {
	want := []domain.PurchaseOrderStatus{
		domain.PurchaseOrderStatusPendente,
		domain.PurchaseOrderStatusRecebido,
		domain.PurchaseOrderStatusCancelado,
	}
	wantStrings := []string{"pendente", "recebido", "cancelado"}

	for i, v := range want {
		v, wantString := v, wantStrings[i]
		t.Run(wantString, func(t *testing.T) {
			if string(v) != wantString {
				t.Errorf("value #%d = %q, want %q", i, string(v), wantString)
			}
			if !v.Valid() {
				t.Errorf("PurchaseOrderStatus(%q).Valid() = false, want true", v)
			}
			parsed, err := domain.ParsePurchaseOrderStatus(string(v))
			if err != nil {
				t.Fatalf("ParsePurchaseOrderStatus(%q) returned unexpected error: %v", v, err)
			}
			if parsed != v {
				t.Errorf("ParsePurchaseOrderStatus(%q) = %q, want %q", v, parsed, v)
			}
		})
	}

	wantSet := map[domain.PurchaseOrderStatus]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range []domain.PurchaseOrderStatus{
		domain.PurchaseOrderStatusPendente, domain.PurchaseOrderStatusRecebido, domain.PurchaseOrderStatusCancelado,
		domain.PurchaseOrderStatus("unknown_candidate_not_in_enum"),
	} {
		if v.Valid() != wantSet[v] {
			t.Errorf("PurchaseOrderStatus(%q).Valid() = %v, want %v (exhaustiveness check)", v, v.Valid(), wantSet[v])
		}
	}
}

func TestPurchaseOrderStatus_ZeroValue_IsInvalid(t *testing.T) {
	var v domain.PurchaseOrderStatus
	if v.Valid() {
		t.Error("zero-value PurchaseOrderStatus.Valid() = true, want false")
	}
}

func TestParsePurchaseOrderStatus_Unknown_ReturnsInvalidKindError(t *testing.T) {
	tests := []string{"", "pending", "PENDENTE", "aberto"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := domain.ParsePurchaseOrderStatus(in)
			if err == nil {
				t.Fatalf("ParsePurchaseOrderStatus(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParsePurchaseOrderStatus(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}

// TestTransactionType_ValidValues_RoundTripAndAreExhaustive mirrors the
// pattern above for TransactionType.
func TestTransactionType_ValidValues_RoundTripAndAreExhaustive(t *testing.T) {
	want := []domain.TransactionType{
		domain.TransactionTypeReceita,
		domain.TransactionTypeDespesa,
	}
	wantStrings := []string{"receita", "despesa"}

	for i, v := range want {
		v, wantString := v, wantStrings[i]
		t.Run(wantString, func(t *testing.T) {
			if string(v) != wantString {
				t.Errorf("value #%d = %q, want %q", i, string(v), wantString)
			}
			if !v.Valid() {
				t.Errorf("TransactionType(%q).Valid() = false, want true", v)
			}
			parsed, err := domain.ParseTransactionType(string(v))
			if err != nil {
				t.Fatalf("ParseTransactionType(%q) returned unexpected error: %v", v, err)
			}
			if parsed != v {
				t.Errorf("ParseTransactionType(%q) = %q, want %q", v, parsed, v)
			}
		})
	}

	wantSet := map[domain.TransactionType]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range []domain.TransactionType{
		domain.TransactionTypeReceita, domain.TransactionTypeDespesa,
		domain.TransactionType("unknown_candidate_not_in_enum"),
	} {
		if v.Valid() != wantSet[v] {
			t.Errorf("TransactionType(%q).Valid() = %v, want %v (exhaustiveness check)", v, v.Valid(), wantSet[v])
		}
	}
}

func TestTransactionType_ZeroValue_IsInvalid(t *testing.T) {
	var v domain.TransactionType
	if v.Valid() {
		t.Error("zero-value TransactionType.Valid() = true, want false")
	}
}

func TestParseTransactionType_Unknown_ReturnsInvalidKindError(t *testing.T) {
	tests := []string{"", "income", "RECEITA", "expense"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := domain.ParseTransactionType(in)
			if err == nil {
				t.Fatalf("ParseTransactionType(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseTransactionType(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}

// TestTransactionCategory_ValidValues_RoundTripAndAreExhaustive mirrors the
// pattern above for TransactionCategory. The 7-value set is pinned down
// exactly as confirmed against mesaposta-frontend's
// src/types/index.ts TransactionCategory union.
func TestTransactionCategory_ValidValues_RoundTripAndAreExhaustive(t *testing.T) {
	want := []domain.TransactionCategory{
		domain.TransactionCategoryVendas,
		domain.TransactionCategoryCompras,
		domain.TransactionCategorySalarios,
		domain.TransactionCategoryAluguel,
		domain.TransactionCategoryManutencao,
		domain.TransactionCategoryMarketing,
		domain.TransactionCategoryOutros,
	}
	wantStrings := []string{
		"vendas", "compras", "salarios", "aluguel", "manutencao", "marketing", "outros",
	}

	for i, v := range want {
		v, wantString := v, wantStrings[i]
		t.Run(wantString, func(t *testing.T) {
			if string(v) != wantString {
				t.Errorf("value #%d = %q, want %q", i, string(v), wantString)
			}
			if !v.Valid() {
				t.Errorf("TransactionCategory(%q).Valid() = false, want true", v)
			}
			parsed, err := domain.ParseTransactionCategory(string(v))
			if err != nil {
				t.Fatalf("ParseTransactionCategory(%q) returned unexpected error: %v", v, err)
			}
			if parsed != v {
				t.Errorf("ParseTransactionCategory(%q) = %q, want %q", v, parsed, v)
			}
		})
	}

	wantSet := map[domain.TransactionCategory]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	candidates := append([]domain.TransactionCategory{}, want...)
	candidates = append(candidates, domain.TransactionCategory("unknown_candidate_not_in_enum"))
	for _, v := range candidates {
		if v.Valid() != wantSet[v] {
			t.Errorf("TransactionCategory(%q).Valid() = %v, want %v (exhaustiveness check)", v, v.Valid(), wantSet[v])
		}
	}
}

func TestTransactionCategory_ZeroValue_IsInvalid(t *testing.T) {
	var v domain.TransactionCategory
	if v.Valid() {
		t.Error("zero-value TransactionCategory.Valid() = true, want false")
	}
}

func TestParseTransactionCategory_Unknown_ReturnsInvalidKindError(t *testing.T) {
	tests := []string{"", "sales", "VENDAS", "impostos"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := domain.ParseTransactionCategory(in)
			if err == nil {
				t.Fatalf("ParseTransactionCategory(%q) returned nil error, want an error", in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("ParseTransactionCategory(%q) error kind = %v, want %v", in, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}
