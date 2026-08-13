package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// ListOrders is the application-layer use case reading every Order visible
// to the caller's tenant, ordered descending by created_at (see
// domain.OrderRepository.FindAll's doc comment).
func ListOrders(ctx context.Context, uow domain.UnitOfWork) ([]*domain.Order, error) {
	var orders []*domain.Order
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Orders().FindAll(ctx)
		if err != nil {
			return err
		}
		orders = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return orders, nil
}
