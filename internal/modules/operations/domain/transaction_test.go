package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewTransaction_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	id := domain.NewID()
	tenantID := mustTenantID(t)
	amount := mustMoney(t, 1500)
	date := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	referenceID := domain.NewID()

	// Act
	tx, err := domain.NewTransaction(
		id, tenantID, domain.TransactionTypeReceita, domain.TransactionCategoryVendas,
		"venda balcao", amount, date, &referenceID,
	)

	// Assert
	if err != nil {
		t.Fatalf("NewTransaction returned unexpected error: %v", err)
	}
	if tx.ID() != id {
		t.Errorf("tx.ID() = %v, want %v", tx.ID(), id)
	}
	if tx.TenantID() != tenantID {
		t.Errorf("tx.TenantID() = %v, want %v", tx.TenantID(), tenantID)
	}
	if tx.TransactionType() != domain.TransactionTypeReceita {
		t.Errorf("tx.TransactionType() = %v, want %v", tx.TransactionType(), domain.TransactionTypeReceita)
	}
	if tx.Category() != domain.TransactionCategoryVendas {
		t.Errorf("tx.Category() = %v, want %v", tx.Category(), domain.TransactionCategoryVendas)
	}
	if tx.Description() != "venda balcao" {
		t.Errorf("tx.Description() = %q, want %q", tx.Description(), "venda balcao")
	}
	if tx.Amount() != amount {
		t.Errorf("tx.Amount() = %v, want %v", tx.Amount(), amount)
	}
	if !tx.Date().Equal(date) {
		t.Errorf("tx.Date() = %v, want %v", tx.Date(), date)
	}
	if tx.ReferenceID() == nil || *tx.ReferenceID() != referenceID {
		t.Errorf("tx.ReferenceID() = %v, want %v", tx.ReferenceID(), referenceID)
	}
}

func TestNewTransaction_NilReferenceID_Succeeds(t *testing.T) {
	// Arrange & Act
	tx, err := domain.NewTransaction(
		domain.NewID(), mustTenantID(t), domain.TransactionTypeDespesa, domain.TransactionCategoryAluguel,
		"aluguel do mes", mustMoney(t, 200000), time.Now(), nil,
	)

	// Assert
	if err != nil {
		t.Fatalf("NewTransaction returned unexpected error: %v", err)
	}
	if tx.ReferenceID() != nil {
		t.Errorf("tx.ReferenceID() = %v, want nil", tx.ReferenceID())
	}
}

func TestNewTransaction_InvalidInput_ReturnsInvalidError(t *testing.T) {
	// Arrange
	validID := domain.NewID()
	validTenantID := mustTenantID(t)
	validType := domain.TransactionTypeReceita
	validCategory := domain.TransactionCategoryVendas
	validDescription := "venda"
	validAmount := mustMoney(t, 100)
	validDate := time.Now()

	tests := []struct {
		name            string
		transactionType domain.TransactionType
		category        domain.TransactionCategory
		description     string
		amount          domain.Money
		date            time.Time
	}{
		{
			name:            "zero amount",
			transactionType: validType,
			category:        validCategory,
			description:     validDescription,
			amount:          mustMoney(t, 0),
			date:            validDate,
		},
		{
			name:            "empty description",
			transactionType: validType,
			category:        validCategory,
			description:     "",
			amount:          validAmount,
			date:            validDate,
		},
		{
			name:            "invalid transaction type",
			transactionType: domain.TransactionType("bogus"),
			category:        validCategory,
			description:     validDescription,
			amount:          validAmount,
			date:            validDate,
		},
		{
			name:            "invalid category",
			transactionType: validType,
			category:        domain.TransactionCategory("bogus"),
			description:     validDescription,
			amount:          validAmount,
			date:            validDate,
		},
		{
			name:            "zero date",
			transactionType: validType,
			category:        validCategory,
			description:     validDescription,
			amount:          validAmount,
			date:            time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := domain.NewTransaction(
				validID, validTenantID, tt.transactionType, tt.category, tt.description, tt.amount, tt.date, nil,
			)

			// Assert
			if err == nil {
				t.Fatalf("NewTransaction(%s) returned nil error, want an error", tt.name)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("NewTransaction(%s) error kind = %v, want %v", tt.name, errs.KindOf(err), errs.Invalid)
			}
		})
	}
}
