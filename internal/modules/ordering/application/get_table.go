package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// GetTable is the application-layer use case reading a single Table by id.
// Like every other getter in this codebase, it still goes through
// UnitOfWork.Do rather than reaching directly for a standalone repository,
// so this read is subject to the same tenant-scoped,
// row-level-security-enforcing database session every write in this module
// uses.
func GetTable(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.Table, error) {
	var table *domain.Table
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Tables().FindByID(ctx, id)
		if err != nil {
			return err
		}
		table = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return table, nil
}
