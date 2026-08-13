package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateTableInput carries everything CreateTable needs to build a
// brand-new Table.
type CreateTableInput struct {
	TenantID tenant.TenantID
	Code     string
	Label    string
}

// CreateTable is the application-layer use case that creates a new Table.
// It builds the aggregate via domain.NewTable -- which performs all field
// validation -- and, only if that succeeds, persists it via
// Tables().Insert within a single UnitOfWork.Do call.
//
// Uniqueness of Code PER TENANT is enforced by a database constraint (see
// internal/platform/db/migrations/0009_ordering.sql), surfaced by
// Tables().Insert as an *errs.Error of Kind errs.Conflict when it fires --
// CreateTable propagates that unchanged.
func CreateTable(ctx context.Context, uow domain.UnitOfWork, input CreateTableInput) (domain.Table, error) {
	table, err := domain.NewTable(domain.NewID(), input.TenantID, input.Code, input.Label)
	if err != nil {
		return domain.Table{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Tables().Insert(ctx, &table)
	})
	if err != nil {
		return domain.Table{}, err
	}

	return table, nil
}
