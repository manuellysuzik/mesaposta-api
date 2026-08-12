package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
)

// GetCustomer is the application-layer use case reading a single Customer
// by id.
//
// It still goes through UnitOfWork.Do rather than reaching directly for a
// standalone repository -- even though this is a plain read with nothing to
// roll back. This is intentional, not an accident: in the real
// Postgres-backed UnitOfWork implementation, Do is where the tenant-scoped,
// row-level-security-enforcing database session gets established (see
// internal/platform/db.Pool.Scope). A getter that bypassed Do to read some
// other way would bypass RLS along with it -- so every read, not just every
// write, is required to flow through the same UnitOfWork/Repositories seam.
// This is also what makes a cross-tenant lookup come back as
// errs.NotFound (never errs.Forbidden): RLS simply hides the row, so
// FindByID reports it exactly like a genuinely nonexistent id.
func GetCustomer(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (*domain.Customer, error) {
	var customer *domain.Customer
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Customers().FindByID(ctx, id)
		if err != nil {
			return err
		}
		customer = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return customer, nil
}
