package http_test

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// fakeStockItemRepository is a func-field test fake for domain.StockItemRepository.
type fakeStockItemRepository struct {
	findByID          func(ctx context.Context, id domain.ID) (*domain.StockItem, error)
	findManyByIDs     func(ctx context.Context, ids []domain.ID) (map[domain.ID]*domain.StockItem, error)
	insert            func(ctx context.Context, item *domain.StockItem) error
	updateWithVersion func(ctx context.Context, item *domain.StockItem, expectedVersion int64) error
}

var _ domain.StockItemRepository = (*fakeStockItemRepository)(nil)

func (f *fakeStockItemRepository) FindByID(ctx context.Context, id domain.ID) (*domain.StockItem, error) {
	if f.findByID == nil {
		return nil, errs.New(errs.NotFound, "stock_item_not_found", "stock item not found", nil)
	}
	return f.findByID(ctx, id)
}

func (f *fakeStockItemRepository) FindManyByIDs(ctx context.Context, ids []domain.ID) (map[domain.ID]*domain.StockItem, error) {
	if f.findManyByIDs == nil {
		return map[domain.ID]*domain.StockItem{}, nil
	}
	return f.findManyByIDs(ctx, ids)
}

func (f *fakeStockItemRepository) Insert(ctx context.Context, item *domain.StockItem) error {
	if f.insert == nil {
		return nil
	}
	return f.insert(ctx, item)
}

func (f *fakeStockItemRepository) UpdateWithVersion(ctx context.Context, item *domain.StockItem, expectedVersion int64) error {
	if f.updateWithVersion == nil {
		return nil
	}
	return f.updateWithVersion(ctx, item, expectedVersion)
}

// fakePurchaseOrderRepository is a func-field test fake for domain.PurchaseOrderRepository.
type fakePurchaseOrderRepository struct {
	findByID          func(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error)
	findByIDForUpdate func(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error)
	insert            func(ctx context.Context, order *domain.PurchaseOrder) error
	updateWithVersion func(ctx context.Context, order *domain.PurchaseOrder, expectedVersion int64) error
}

var _ domain.PurchaseOrderRepository = (*fakePurchaseOrderRepository)(nil)

func (f *fakePurchaseOrderRepository) FindByID(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	if f.findByID == nil {
		return nil, errs.New(errs.NotFound, "purchase_order_not_found", "purchase order not found", nil)
	}
	return f.findByID(ctx, id)
}

func (f *fakePurchaseOrderRepository) FindByIDForUpdate(ctx context.Context, id domain.ID) (*domain.PurchaseOrder, error) {
	if f.findByIDForUpdate == nil {
		return nil, errs.New(errs.NotFound, "purchase_order_not_found", "purchase order not found", nil)
	}
	return f.findByIDForUpdate(ctx, id)
}

func (f *fakePurchaseOrderRepository) Insert(ctx context.Context, order *domain.PurchaseOrder) error {
	if f.insert == nil {
		return nil
	}
	return f.insert(ctx, order)
}

func (f *fakePurchaseOrderRepository) UpdateWithVersion(ctx context.Context, order *domain.PurchaseOrder, expectedVersion int64) error {
	if f.updateWithVersion == nil {
		return nil
	}
	return f.updateWithVersion(ctx, order, expectedVersion)
}

// fakeStockMovementRepository is a func-field test fake for domain.StockMovementRepository.
type fakeStockMovementRepository struct {
	insert func(ctx context.Context, movement domain.StockMovement) error
}

var _ domain.StockMovementRepository = (*fakeStockMovementRepository)(nil)

func (f *fakeStockMovementRepository) Insert(ctx context.Context, movement domain.StockMovement) error {
	if f.insert == nil {
		return nil
	}
	return f.insert(ctx, movement)
}

// fakeTransactionRepository is a func-field test fake for domain.TransactionRepository.
type fakeTransactionRepository struct {
	insert func(ctx context.Context, txn domain.Transaction) error
}

var _ domain.TransactionRepository = (*fakeTransactionRepository)(nil)

func (f *fakeTransactionRepository) Insert(ctx context.Context, txn domain.Transaction) error {
	if f.insert == nil {
		return nil
	}
	return f.insert(ctx, txn)
}

// fakeEventPublisher is a func-field test fake for outbox.DomainEventPublisher.
type fakeEventPublisher struct {
	publish func(ctx context.Context, events ...outbox.DomainEvent) error
}

var _ outbox.DomainEventPublisher = (*fakeEventPublisher)(nil)

func (f *fakeEventPublisher) Publish(ctx context.Context, events ...outbox.DomainEvent) error {
	if f.publish == nil {
		return nil
	}
	return f.publish(ctx, events...)
}

// fakeAuditRecorder is a func-field test fake for audit.Recorder. Its
// default (nil record func) accumulates every entry into recorded, so a
// handler test that does not care about auditing keeps passing untouched
// while one that does can assert on what was written.
type fakeAuditRecorder struct {
	record   func(ctx context.Context, entries ...audit.Entry) error
	recorded []audit.Entry
}

var _ audit.Recorder = (*fakeAuditRecorder)(nil)

func (f *fakeAuditRecorder) Record(ctx context.Context, entries ...audit.Entry) error {
	f.recorded = append(f.recorded, entries...)
	if f.record == nil {
		return nil
	}
	return f.record(ctx, entries...)
}

// fakeRepositories is a func-field test fake for domain.Repositories.
type fakeRepositories struct {
	stockItems     *fakeStockItemRepository
	purchaseOrders *fakePurchaseOrderRepository
	stockMovements *fakeStockMovementRepository
	transactions   *fakeTransactionRepository
	events         *fakeEventPublisher
	auditEntries   *fakeAuditRecorder
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func newFakeRepositories() *fakeRepositories {
	return &fakeRepositories{
		stockItems:     &fakeStockItemRepository{},
		purchaseOrders: &fakePurchaseOrderRepository{},
		stockMovements: &fakeStockMovementRepository{},
		transactions:   &fakeTransactionRepository{},
		events:         &fakeEventPublisher{},
		auditEntries:   &fakeAuditRecorder{},
	}
}

func (f *fakeRepositories) StockItems() domain.StockItemRepository         { return f.stockItems }
func (f *fakeRepositories) PurchaseOrders() domain.PurchaseOrderRepository { return f.purchaseOrders }
func (f *fakeRepositories) StockMovements() domain.StockMovementRepository { return f.stockMovements }
func (f *fakeRepositories) Transactions() domain.TransactionRepository     { return f.transactions }
func (f *fakeRepositories) Events() outbox.DomainEventPublisher            { return f.events }
func (f *fakeRepositories) Audit() audit.Recorder                          { return f.auditEntries }

// fakeUnitOfWork is a test fake for domain.UnitOfWork.
type fakeUnitOfWork struct {
	repos *fakeRepositories
}

var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

func (f *fakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	return fn(ctx, f.repos)
}

// newFakeUnitOfWork returns a UnitOfWork fake wired to a fresh fakeRepositories
// with no-op defaults. Callers reach into fake.repos.<repo>.<method> to
// configure specific test-case behavior.
func newFakeUnitOfWork() *fakeUnitOfWork {
	return &fakeUnitOfWork{repos: newFakeRepositories()}
}
