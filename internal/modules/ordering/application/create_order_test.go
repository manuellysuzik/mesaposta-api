package application_test

import (
	"context"
	"testing"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// seedTableSession creates an active table + a valid session bound to it in
// state, returning the table and the session's PLAINTEXT token.
func seedTableSession(t *testing.T, state *fakeState) (*domain.Table, string) {
	t.Helper()
	table := mustFakeTable(t)
	state.tables[table.ID()] = table

	session, plaintext, err := domain.NewTableSession(mustFakeUOWTenantID(t), table.ID())
	if err != nil {
		t.Fatalf("NewTableSession: %v", err)
	}
	state.tableSessions[session.TokenHash()] = session
	return table, plaintext
}

func TestCreateOrder_Valid_CreatesPendingOrderWithFrozenPrices(t *testing.T) {
	state := newFakeState()
	_, token := seedTableSession(t, state)
	uow := newFakeUnitOfWork(state)

	reader := newFakePriceReader()
	reader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 1500, IsActive: true}

	name := "Cliente Balcão"
	order, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: token,
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 2}},
		CustomerName: &name,
	})
	if err != nil {
		t.Fatalf("CreateOrder: unexpected error: %v", err)
	}
	if order.Status() != domain.OrderStatusPendenteConfirmacao {
		t.Errorf("Status() = %q, want %q", order.Status(), domain.OrderStatusPendenteConfirmacao)
	}
	items := order.Items()
	if len(items) != 1 || items[0].UnitPrice().Cents() != 1500 || items[0].Quantity().Count() != 2 {
		t.Errorf("Items() = %+v, want one item p1/1500/2", items)
	}
	if _, ok := uow.snapshot().orders[order.ID()]; !ok {
		t.Error("order was not persisted")
	}
}

func TestCreateOrder_EmptySessionToken_ReturnsUnauthenticated(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	reader := newFakePriceReader()

	_, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: "",
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 1}},
	})
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Unauthenticated", errs.KindOf(err))
	}
}

func TestCreateOrder_UnknownSessionToken_ReturnsUnauthenticatedAndPersistsNothing(t *testing.T) {
	state := newFakeState()
	uow := newFakeUnitOfWork(state)
	reader := newFakePriceReader()
	reader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 500, IsActive: true}

	_, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: "totally-bogus-token",
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 1}},
	})
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Unauthenticated", errs.KindOf(err))
	}
	if len(uow.snapshot().orders) != 0 {
		t.Error("CreateOrder persisted an order despite an invalid session token")
	}
}

func TestCreateOrder_InactiveTable_ReturnsConflictAndPersistsNothing(t *testing.T) {
	state := newFakeState()
	table, token := seedTableSession(t, state)
	table.Deactivate()
	uow := newFakeUnitOfWork(state)

	reader := newFakePriceReader()
	reader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 500, IsActive: true}

	_, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: token,
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 1}},
	})
	if !errs.IsKind(err, errs.Conflict) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
	if len(uow.snapshot().orders) != 0 {
		t.Error("CreateOrder persisted an order despite an inactive table")
	}
}

func TestCreateOrder_ProductNotFoundInCatalog_ReturnsInvalidAndPersistsNothing(t *testing.T) {
	state := newFakeState()
	_, token := seedTableSession(t, state)
	uow := newFakeUnitOfWork(state)
	reader := newFakePriceReader() // no prices seeded: every lookup is NotFound

	_, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: token,
		Items:        []application.CreateOrderItemInput{{ProductID: "ghost", QuantityCount: 1}},
	})
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
	if len(uow.snapshot().orders) != 0 {
		t.Error("CreateOrder persisted an order despite an unpriceable item")
	}
}

func TestCreateOrder_ProductInactive_ReturnsInvalid(t *testing.T) {
	state := newFakeState()
	_, token := seedTableSession(t, state)
	uow := newFakeUnitOfWork(state)
	reader := newFakePriceReader()
	reader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 500, IsActive: false}

	_, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: token,
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 1}},
	})
	if !errs.IsKind(err, errs.Invalid) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

// TestCreateOrder_ClientSuppliedPriceIsIgnored proves the module's core
// security invariant: even though CreateOrderItemInput has NO price field
// at all (so a malicious client literally cannot submit one), this test
// pins down that the price actually persisted always comes from
// priceReader, never anything client-controllable -- by seeding a price
// via the reader and confirming the persisted line matches THAT, not some
// other value.
func TestCreateOrder_ClientSuppliedPriceIsIgnored(t *testing.T) {
	state := newFakeState()
	_, token := seedTableSession(t, state)
	uow := newFakeUnitOfWork(state)
	reader := newFakePriceReader()
	reader.prices["p1"] = catalogcontracts.ProductPrice{ProductID: "p1", PriceCents: 12345, IsActive: true}

	order, err := application.CreateOrder(context.Background(), uow, reader, application.CreateOrderInput{
		TenantID:     mustFakeUOWTenantID(t),
		SessionToken: token,
		Items:        []application.CreateOrderItemInput{{ProductID: "p1", QuantityCount: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: unexpected error: %v", err)
	}
	if got := order.Items()[0].UnitPrice().Cents(); got != 12345 {
		t.Errorf("UnitPrice().Cents() = %d, want 12345 (from priceReader)", got)
	}
}
