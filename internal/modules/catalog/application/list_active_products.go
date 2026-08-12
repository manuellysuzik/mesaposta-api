package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
)

// ListActiveProducts is the application-layer use case reading every
// active Product visible to the caller's tenant, ordered ascending by
// name (see domain.ProductRepository.FindActive's doc comment). Like
// GetProduct, it goes through UnitOfWork.Do rather than a standalone
// repository so this read is subject to the same tenant-scoped,
// row-level-security-enforcing database session every other access in
// this module uses.
func ListActiveProducts(ctx context.Context, uow domain.UnitOfWork) ([]*domain.Product, error) {
	var products []*domain.Product
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Products().FindActive(ctx)
		if err != nil {
			return err
		}
		products = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return products, nil
}
