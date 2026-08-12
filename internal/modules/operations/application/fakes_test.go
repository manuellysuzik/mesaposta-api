package application_test

import (
	"context"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// This file defines a hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories, following the same
// fake-implements-the-port pattern internal/platform/auth/refresh's own
// RefreshStore fake (refresh_test.go's memStore) already establishes for
// this project. It is intentionally unexported and lives only in this
// (application_test) test package -- nothing in application/'s production
// code, nor any other package, can see or depend on it.
//
// The single most important property this fake must have -- the one the
// rest of this package's tests lean on to prove atomicity without a real
// Postgres transaction -- is that fakeUnitOfWork.Do simulates a genuine
// rollback: it operates on a CLONE of its state, and only replaces the
// real state with that clone if fn returns nil. See
// TestFakeUnitOfWork_Do_RollsBackOnError below, which pins that behavior
// down directly, with nothing to do with ReceivePurchaseOrder or any other
// use case.

// fakeState is the mutable data a fakeUnitOfWork operates over.
type fakeState struct {
	stockItems     map[domain.ID]*domain.StockItem
	purchaseOrders map[domain.ID]*domain.PurchaseOrder
	stockMovements []domain.StockMovement
	transactions   []domain.Transaction
	events         []outbox.DomainEvent
}

// newFakeState returns an empty, ready-to-use fakeState.
func newFakeState() *fakeState {
	return &fakeState{
		stockItems:     map[domain.ID]*domain.StockItem{},
		purchaseOrders: map[domain.ID]*domain.PurchaseOrder{},
	}
}

// clone returns an independent copy of s: new maps and slices, and new
// copies of every pointed-to aggregate, so that mutating a *StockItem or
// *PurchaseOrder obtained from the clone during a Do call -- including one
// that is ultimately rolled back -- can never reach back into s itself.
func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		stockItems:     make(map[domain.ID]*domain.StockItem, len(s.stockItems)),
		purchaseOrders: make(map[domain.ID]*domain.PurchaseOrder, len(s.purchaseOrders)),
		stockMovements: append([]domain.StockMovement{}, s.stockMovements...),
		transactions:   append([]domain.Transaction{}, s.transactions...),
		events:         append([]outbox.DomainEvent{}, s.events...),
	}
	for id, item := range s.stockItems {
		itemCopy := *item
		clone.stockItems[id] = &itemCopy
	}
	for id, order := range s.purchaseOrders {
		orderCopy := *order
		clone.purchaseOrders[id] = &orderCopy
	}
	return clone
}

// fakeRepoErrors lets a test inject a canned error to be returned by a
// specific repository method, on every call, for the duration of a single
// fakeUnitOfWork.Do invocation -- used to simulate failure modes (e.g. an
// optimistic-concurrency conflict on PurchaseOrders().UpdateWithVersion)
// that would otherwise require a real second, concurrent transaction to
// reproduce.
type fakeRepoErrors struct {
	stockItemFindByID              error
	stockItemFindManyByIDs         error
	stockItemInsert                error
	stockItemUpdateWithVersion     error
	purchaseOrderFindByID          error
	purchaseOrderFindByIDForUpdate error
	purchaseOrderInsert            error
	purchaseOrderUpdateWithVersion error
	stockMovementInsert            error
	transactionInsert              error
	eventsPublish                  error
}

// fakeUnitOfWork is an in-memory domain.UnitOfWork. Its zero value is not
// usable; construct one via newFakeUnitOfWork.
type fakeUnitOfWork struct {
	mu    sync.Mutex
	state *fakeState

	// errors is consulted by every repository method built on top of this
	// fakeUnitOfWork, for every Do call, letting a test set it up once
	// before calling the use case under test.
	errors fakeRepoErrors

	// trail records, in order, the name of every write attempted through
	// any Repositories obtained from Do -- across every Do call made on
	// this fakeUnitOfWork, whether that call ultimately committed or
	// rolled back. It exists purely so tests can assert on write order
	// (see TestReceivePurchaseOrder_PersistsPurchaseOrderBeforeAnyOtherWrite).
	trailMu sync.Mutex
	trail   []string
}

// newFakeUnitOfWork returns a fakeUnitOfWork seeded with state.
func newFakeUnitOfWork(state *fakeState) *fakeUnitOfWork {
	return &fakeUnitOfWork{state: state}
}

// compile-time check: *fakeUnitOfWork must satisfy domain.UnitOfWork.
var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

// Do clones u's current state, runs fn against Repositories backed by that
// clone, and -- only if fn returns nil -- replaces u's real state with the
// clone. If fn returns a non-nil error, the clone is discarded and u's real
// state is left exactly as it was before Do was called: a rollback.
func (u *fakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	u.mu.Lock()
	clone := u.state.clone()
	u.mu.Unlock()

	repos := &fakeRepositories{uow: u, state: clone}

	if err := fn(ctx, repos); err != nil {
		return err
	}

	u.mu.Lock()
	u.state = clone
	u.mu.Unlock()
	return nil
}

// snapshot returns a clone of u's current, real (committed) state -- for
// test assertions only.
func (u *fakeUnitOfWork) snapshot() *fakeState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.clone()
}

// recordTrail appends name to u's write trail.
func (u *fakeUnitOfWork) recordTrail(name string) {
	u.trailMu.Lock()
	defer u.trailMu.Unlock()
	u.trail = append(u.trail, name)
}

// Trail returns a defensive copy of every write attempted so far.
func (u *fakeUnitOfWork) Trail() []string {
	u.trailMu.Lock()
	defer u.trailMu.Unlock()
	out := make([]string, len(u.trail))
	copy(out, u.trail)
	return out
}

// fakeRepositories is the domain.Repositories implementation handed to fn
// by fakeUnitOfWork.Do, scoped to a single clone of state.
type fakeRepositories struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (r *fakeRepositories) StockItems() domain.StockItemRepository {
	return &fakeStockItemRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) PurchaseOrders() domain.PurchaseOrderRepository {
	return &fakePurchaseOrderRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) StockMovements() domain.StockMovementRepository {
	return &fakeStockMovementRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Transactions() domain.TransactionRepository {
	return &fakeTransactionRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{uow: r.uow, state: r.state}
}

// --- StockItemRepository -------------------------------------------------

type fakeStockItemRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.StockItemRepository = (*fakeStockItemRepository)(nil)

func (r *fakeStockItemRepository) FindByID(_ context.Context, id domain.ID) (*domain.StockItem, error) {
	if err := r.uow.errors.stockItemFindByID; err != nil {
		return nil, err
	}
	item, ok := r.state.stockItems[id]
	if !ok {
		return nil, notFoundErr("stock_item_not_found", "stock item not found")
	}
	return item, nil
}

func (r *fakeStockItemRepository) FindManyByIDs(_ context.Context, ids []domain.ID) (map[domain.ID]*domain.StockItem, error) {
	if err := r.uow.errors.stockItemFindManyByIDs; err != nil {
		return nil, err
	}
	out := make(map[domain.ID]*domain.StockItem, len(ids))
	for _, id := range ids {
		if item, ok := r.state.stockItems[id]; ok {
			out[id] = item
		}
	}
	return out, nil
}

func (r *fakeStockItemRepository) Insert(_ context.Context, item *domain.StockItem) error {
	r.uow.recordTrail("StockItems.Insert")
	if err := r.uow.errors.stockItemInsert; err != nil {
		return err
	}
	r.state.stockItems[item.ID()] = item
	return nil
}

func (r *fakeStockItemRepository) UpdateWithVersion(_ context.Context, item *domain.StockItem, _ int64) error {
	r.uow.recordTrail("StockItems.UpdateWithVersion")
	if err := r.uow.errors.stockItemUpdateWithVersion; err != nil {
		return err
	}
	r.state.stockItems[item.ID()] = item
	return nil
}

// --- PurchaseOrderRepository ----------------------------------------------

type fakePurchaseOrderRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.PurchaseOrderRepository = (*fakePurchaseOrderRepository)(nil)

func (r *fakePurchaseOrderRepository) FindByID(_ context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	if err := r.uow.errors.purchaseOrderFindByID; err != nil {
		return nil, err
	}
	order, ok := r.state.purchaseOrders[id]
	if !ok {
		return nil, notFoundErr("purchase_order_not_found", "purchase order not found")
	}
	return order, nil
}

func (r *fakePurchaseOrderRepository) FindByIDForUpdate(_ context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	if err := r.uow.errors.purchaseOrderFindByIDForUpdate; err != nil {
		return nil, err
	}
	order, ok := r.state.purchaseOrders[id]
	if !ok {
		return nil, notFoundErr("purchase_order_not_found", "purchase order not found")
	}
	return order, nil
}

func (r *fakePurchaseOrderRepository) Insert(_ context.Context, order *domain.PurchaseOrder) error {
	r.uow.recordTrail("PurchaseOrders.Insert")
	if err := r.uow.errors.purchaseOrderInsert; err != nil {
		return err
	}
	r.state.purchaseOrders[order.ID()] = order
	return nil
}

func (r *fakePurchaseOrderRepository) UpdateWithVersion(_ context.Context, order *domain.PurchaseOrder, _ int64) error {
	r.uow.recordTrail("PurchaseOrders.UpdateWithVersion")
	if err := r.uow.errors.purchaseOrderUpdateWithVersion; err != nil {
		return err
	}
	r.state.purchaseOrders[order.ID()] = order
	return nil
}

// --- StockMovementRepository -----------------------------------------------

type fakeStockMovementRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.StockMovementRepository = (*fakeStockMovementRepository)(nil)

func (r *fakeStockMovementRepository) Insert(_ context.Context, movement domain.StockMovement) error {
	r.uow.recordTrail("StockMovements.Insert")
	if err := r.uow.errors.stockMovementInsert; err != nil {
		return err
	}
	r.state.stockMovements = append(r.state.stockMovements, movement)
	return nil
}

// --- TransactionRepository -------------------------------------------------

type fakeTransactionRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.TransactionRepository = (*fakeTransactionRepository)(nil)

func (r *fakeTransactionRepository) Insert(_ context.Context, transaction domain.Transaction) error {
	r.uow.recordTrail("Transactions.Insert")
	if err := r.uow.errors.transactionInsert; err != nil {
		return err
	}
	r.state.transactions = append(r.state.transactions, transaction)
	return nil
}

// --- DomainEventPublisher ---------------------------------------------------

type fakeEventPublisher struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ outbox.DomainEventPublisher = (*fakeEventPublisher)(nil)

func (p *fakeEventPublisher) Publish(_ context.Context, events ...outbox.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	p.uow.recordTrail("Events.Publish")
	if err := p.uow.errors.eventsPublish; err != nil {
		return err
	}
	p.state.events = append(p.state.events, events...)
	return nil
}

// notFoundErr builds the *errs.Error the fake repositories return for a
// missing lookup.
func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}
