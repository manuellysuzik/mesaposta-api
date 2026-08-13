package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// ListTables is the application-layer use case reading every Table visible
// to the caller's tenant, ordered ascending by code (see
// domain.TableRepository.FindAll's doc comment).
func ListTables(ctx context.Context, uow domain.UnitOfWork) ([]*domain.Table, error) {
	var tables []*domain.Table
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Tables().FindAll(ctx)
		if err != nil {
			return err
		}
		tables = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tables, nil
}
