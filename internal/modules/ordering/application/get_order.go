package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// GetOrder is the application-layer use case reading a single Order by id.
func GetOrder(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.Order, error) {
	var order *domain.Order
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Orders().FindByID(ctx, id)
		if err != nil {
			return err
		}
		order = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}
