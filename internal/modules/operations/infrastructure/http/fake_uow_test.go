package http_test

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// fakeRepositories is a test fake for domain.Repositories with func fields.
type fakeRepositories struct {
	findStockItemByID     func(ctx context.Context, tenantID domain.TenantID, id domain.ID) (*domain.StockItem, error)
	saveStockItem         func(ctx context.Context, item *domain.StockItem) error
	findPurchaseOrderByID func(ctx context.Context, tenantID domain.TenantID, id domain.ID) (*domain.PurchaseOrder, error)
	savePurchaseOrder     func(ctx context.Context, order *domain.PurchaseOrder) error
	saveStockMovement     func(ctx context.Context, movement *domain.StockMovement) error
	saveTransaction       func(ctx context.Context, txn *domain.Transaction) error
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (f *fakeRepositories) FindStockItemByID(ctx context.Context, tenantID domain.TenantID, id domain.ID) (*domain.StockItem, error) {
	if f.findStockItemByID == nil {
		return nil, errs.New(errs.Internal, "not_configured", "findStockItemByID not configured in fake", nil)
	}
	return f.findStockItemByID(ctx, tenantID, id)
}

func (f *fakeRepositories) SaveStockItem(ctx context.Context, item *domain.StockItem) error {
	if f.saveStockItem == nil {
		return errs.New(errs.Internal, "not_configured", "saveStockItem not configured in fake", nil)
	}
	return f.saveStockItem(ctx, item)
}

func (f *fakeRepositories) FindPurchaseOrderByID(ctx context.Context, tenantID domain.TenantID, id domain.ID) (*domain.PurchaseOrder, error) {
	if f.findPurchaseOrderByID == nil {
		return nil, errs.New(errs.Internal, "not_configured", "findPurchaseOrderByID not configured in fake", nil)
	}
	return f.findPurchaseOrderByID(ctx, tenantID, id)
}

func (f *fakeRepositories) SavePurchaseOrder(ctx context.Context, order *domain.PurchaseOrder) error {
	if f.savePurchaseOrder == nil {
		return errs.New(errs.Internal, "not_configured", "savePurchaseOrder not configured in fake", nil)
	}
	return f.savePurchaseOrder(ctx, order)
}

func (f *fakeRepositories) SaveStockMovement(ctx context.Context, movement *domain.StockMovement) error {
	if f.saveStockMovement == nil {
		return errs.New(errs.Internal, "not_configured", "saveStockMovement not configured in fake", nil)
	}
	return f.saveStockMovement(ctx, movement)
}

func (f *fakeRepositories) SaveTransaction(ctx context.Context, txn *domain.Transaction) error {
	if f.saveTransaction == nil {
		return errs.New(errs.Internal, "not_configured", "saveTransaction not configured in fake", nil)
	}
	return f.saveTransaction(ctx, txn)
}

// fakeUnitOfWork is a test fake for domain.UnitOfWork.
type fakeUnitOfWork struct {
	doFn func(ctx context.Context, fn func(domain.Repositories) error) error
}

var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

func (f *fakeUnitOfWork) Do(ctx context.Context, fn func(domain.Repositories) error) error {
	if f.doFn == nil {
		return errs.New(errs.Internal, "not_configured", "Do not configured in fake", nil)
	}
	return f.doFn(ctx, fn)
}

// newFakeUnitOfWork returns a UnitOfWork fake with no-op defaults.
// Callers set the func fields as needed for each test case.
func newFakeUnitOfWork() *fakeUnitOfWork {
	return &fakeUnitOfWork{
		doFn: func(ctx context.Context, fn func(domain.Repositories) error) error {
			return fn(&fakeRepositories{})
		},
	}
}
