package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Each enum below follows the same closed-set pattern as
// internal/platform/auth/token.Role: a string-based type whose zero value
// ("") is deliberately invalid, a Valid method that reports membership in
// the known set, and a Parse<Name> constructor that rejects any unknown
// (including empty) value with an *errs.Error of Kind errs.Invalid instead
// of letting it flow through as a silent, unvalidated default.

// StockUnit identifies the unit of measure for a stock item.
type StockUnit string

const (
	// StockUnitKg is kilograms.
	StockUnitKg StockUnit = "kg"
	// StockUnitG is grams.
	StockUnitG StockUnit = "g"
	// StockUnitL is liters.
	StockUnitL StockUnit = "l"
	// StockUnitMl is milliliters.
	StockUnitMl StockUnit = "ml"
	// StockUnitUn is a discrete unit (unidade).
	StockUnitUn StockUnit = "un"
)

// Valid reports whether u is one of the closed set of known stock units.
func (u StockUnit) Valid() bool {
	switch u {
	case StockUnitKg, StockUnitG, StockUnitL, StockUnitMl, StockUnitUn:
		return true
	default:
		return false
	}
}

// ParseStockUnit validates s against the closed set of known stock units.
func ParseStockUnit(s string) (StockUnit, error) {
	u := StockUnit(s)
	if !u.Valid() {
		return "", errs.New(errs.Invalid, "invalid_stock_unit", "unknown stock unit",
			fmt.Errorf("stock unit %q is not one of the known values", s))
	}
	return u, nil
}

// StockMovementType identifies whether a stock movement increases or
// decreases the affected stock item's quantity.
type StockMovementType string

const (
	// StockMovementTypeEntrada is an inbound movement (received, adjusted
	// up).
	StockMovementTypeEntrada StockMovementType = "entrada"
	// StockMovementTypeSaida is an outbound movement (sold, adjusted down).
	StockMovementTypeSaida StockMovementType = "saida"
)

// Valid reports whether t is one of the closed set of known stock
// movement types.
func (t StockMovementType) Valid() bool {
	switch t {
	case StockMovementTypeEntrada, StockMovementTypeSaida:
		return true
	default:
		return false
	}
}

// ParseStockMovementType validates s against the closed set of known stock
// movement types.
func ParseStockMovementType(s string) (StockMovementType, error) {
	t := StockMovementType(s)
	if !t.Valid() {
		return "", errs.New(errs.Invalid, "invalid_stock_movement_type", "unknown stock movement type",
			fmt.Errorf("stock movement type %q is not one of the known values", s))
	}
	return t, nil
}

// PurchaseOrderStatus identifies the lifecycle state of a purchase order.
type PurchaseOrderStatus string

const (
	// PurchaseOrderStatusPendente is a purchase order awaiting receipt.
	PurchaseOrderStatusPendente PurchaseOrderStatus = "pendente"
	// PurchaseOrderStatusRecebido is a purchase order that has been
	// received, closing the loop with the stock movements it generated.
	PurchaseOrderStatusRecebido PurchaseOrderStatus = "recebido"
	// PurchaseOrderStatusCancelado is a purchase order that was cancelled
	// before receipt.
	PurchaseOrderStatusCancelado PurchaseOrderStatus = "cancelado"
)

// Valid reports whether s is one of the closed set of known purchase
// order statuses.
func (s PurchaseOrderStatus) Valid() bool {
	switch s {
	case PurchaseOrderStatusPendente, PurchaseOrderStatusRecebido, PurchaseOrderStatusCancelado:
		return true
	default:
		return false
	}
}

// ParsePurchaseOrderStatus validates s against the closed set of known
// purchase order statuses.
func ParsePurchaseOrderStatus(s string) (PurchaseOrderStatus, error) {
	status := PurchaseOrderStatus(s)
	if !status.Valid() {
		return "", errs.New(errs.Invalid, "invalid_purchase_order_status", "unknown purchase order status",
			fmt.Errorf("purchase order status %q is not one of the known values", s))
	}
	return status, nil
}

// TransactionType identifies whether a financial transaction is inbound
// (revenue) or outbound (expense).
type TransactionType string

const (
	// TransactionTypeReceita is a revenue (inbound) transaction.
	TransactionTypeReceita TransactionType = "receita"
	// TransactionTypeDespesa is an expense (outbound) transaction.
	TransactionTypeDespesa TransactionType = "despesa"
)

// Valid reports whether t is one of the closed set of known transaction
// types.
func (t TransactionType) Valid() bool {
	switch t {
	case TransactionTypeReceita, TransactionTypeDespesa:
		return true
	default:
		return false
	}
}

// ParseTransactionType validates s against the closed set of known
// transaction types.
func ParseTransactionType(s string) (TransactionType, error) {
	t := TransactionType(s)
	if !t.Valid() {
		return "", errs.New(errs.Invalid, "invalid_transaction_type", "unknown transaction type",
			fmt.Errorf("transaction type %q is not one of the known values", s))
	}
	return t, nil
}

// TransactionCategory classifies a financial transaction. This set was
// confirmed against mesaposta-frontend's src/types/index.ts
// TransactionCategory union (see enums_test.go).
type TransactionCategory string

const (
	// TransactionCategoryVendas is revenue from sales.
	TransactionCategoryVendas TransactionCategory = "vendas"
	// TransactionCategoryCompras is spend on purchases (e.g. purchase
	// orders).
	TransactionCategoryCompras TransactionCategory = "compras"
	// TransactionCategorySalarios is spend on salaries.
	TransactionCategorySalarios TransactionCategory = "salarios"
	// TransactionCategoryAluguel is spend on rent.
	TransactionCategoryAluguel TransactionCategory = "aluguel"
	// TransactionCategoryManutencao is spend on maintenance.
	TransactionCategoryManutencao TransactionCategory = "manutencao"
	// TransactionCategoryMarketing is spend on marketing.
	TransactionCategoryMarketing TransactionCategory = "marketing"
	// TransactionCategoryOutros is any category not covered above.
	TransactionCategoryOutros TransactionCategory = "outros"
)

// Valid reports whether c is one of the closed set of known transaction
// categories.
func (c TransactionCategory) Valid() bool {
	switch c {
	case TransactionCategoryVendas, TransactionCategoryCompras, TransactionCategorySalarios,
		TransactionCategoryAluguel, TransactionCategoryManutencao, TransactionCategoryMarketing,
		TransactionCategoryOutros:
		return true
	default:
		return false
	}
}

// ParseTransactionCategory validates s against the closed set of known
// transaction categories.
func ParseTransactionCategory(s string) (TransactionCategory, error) {
	c := TransactionCategory(s)
	if !c.Valid() {
		return "", errs.New(errs.Invalid, "invalid_transaction_category", "unknown transaction category",
			fmt.Errorf("transaction category %q is not one of the known values", s))
	}
	return c, nil
}
