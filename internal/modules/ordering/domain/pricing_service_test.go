package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestPriceOrderLines_Empty_ReturnsEmptySlice(t *testing.T) {
	items, err := domain.PriceOrderLines(nil, map[string]domain.PriceSnapshot{})
	if err != nil {
		t.Fatalf("PriceOrderLines: unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestPriceOrderLines_Valid_FreezesSnapshotPrice(t *testing.T) {
	lines := []domain.RequestedLine{
		{ProductID: "p1", QuantityCount: 2},
		{ProductID: "p2", QuantityCount: 1},
	}
	prices := map[string]domain.PriceSnapshot{
		"p1": {PriceCents: 1500, Active: true},
		"p2": {PriceCents: 700, Active: true},
	}

	items, err := domain.PriceOrderLines(lines, prices)
	if err != nil {
		t.Fatalf("PriceOrderLines: unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ProductID() != "p1" || items[0].UnitPrice().Cents() != 1500 || items[0].Quantity().Count() != 2 {
		t.Errorf("items[0] = %+v, want p1/1500/2", items[0])
	}
	if items[1].ProductID() != "p2" || items[1].UnitPrice().Cents() != 700 || items[1].Quantity().Count() != 1 {
		t.Errorf("items[1] = %+v, want p2/700/1", items[1])
	}
}

func TestPriceOrderLines_ProductNotInSnapshot_ReturnsInvalid(t *testing.T) {
	lines := []domain.RequestedLine{{ProductID: "missing", QuantityCount: 1}}

	_, err := domain.PriceOrderLines(lines, map[string]domain.PriceSnapshot{})
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestPriceOrderLines_ProductInactive_ReturnsInvalid(t *testing.T) {
	lines := []domain.RequestedLine{{ProductID: "p1", QuantityCount: 1}}
	prices := map[string]domain.PriceSnapshot{"p1": {PriceCents: 500, Active: false}}

	_, err := domain.PriceOrderLines(lines, prices)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestPriceOrderLines_ZeroQuantity_ReturnsInvalid(t *testing.T) {
	lines := []domain.RequestedLine{{ProductID: "p1", QuantityCount: 0}}
	prices := map[string]domain.PriceSnapshot{"p1": {PriceCents: 500, Active: true}}

	_, err := domain.PriceOrderLines(lines, prices)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestPriceOrderLines_NegativePrice_ReturnsInvalid(t *testing.T) {
	lines := []domain.RequestedLine{{ProductID: "p1", QuantityCount: 1}}
	prices := map[string]domain.PriceSnapshot{"p1": {PriceCents: -100, Active: true}}

	_, err := domain.PriceOrderLines(lines, prices)
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
