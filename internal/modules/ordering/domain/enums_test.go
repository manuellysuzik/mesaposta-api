package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestOrderStatus_Valid(t *testing.T) {
	valid := []domain.OrderStatus{
		domain.OrderStatusPendenteConfirmacao, domain.OrderStatusConfirmado,
		domain.OrderStatusEmPreparo, domain.OrderStatusPronto,
		domain.OrderStatusEntregue, domain.OrderStatusCancelado,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("OrderStatus(%q).Valid() = false, want true", s)
		}
	}
	if domain.OrderStatus("bogus").Valid() {
		t.Error(`OrderStatus("bogus").Valid() = true, want false`)
	}
	if domain.OrderStatus("").Valid() {
		t.Error(`OrderStatus("").Valid() = true, want false`)
	}
}

func TestParseOrderStatus(t *testing.T) {
	got, err := domain.ParseOrderStatus("confirmado")
	if err != nil {
		t.Fatalf("ParseOrderStatus: unexpected error: %v", err)
	}
	if got != domain.OrderStatusConfirmado {
		t.Errorf("ParseOrderStatus = %q, want %q", got, domain.OrderStatusConfirmado)
	}

	_, err = domain.ParseOrderStatus("nao-existe")
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
