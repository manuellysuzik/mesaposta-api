package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateCustomerInput carries everything CreateCustomer needs to build a
// brand-new Customer.
type CreateCustomerInput struct {
	TenantID tenant.TenantID
	Name     string
	Email    *string
	Phone    *string
}

// CreateCustomer is the application-layer use case that creates a new
// Customer. It builds the aggregate via domain.NewCustomer -- which
// performs all field validation -- and, only if that succeeds, persists it
// via Customers().Insert within a single UnitOfWork.Do call.
//
// Email/phone uniqueness PER TENANT (not globally) is enforced by a
// database constraint (see
// internal/platform/db/migrations/0006_crm.sql), surfaced by
// Customers().Insert as an *errs.Error of Kind errs.Conflict when it fires
// -- CreateCustomer propagates that unchanged, it does not attempt its own
// pre-check (which would be redundant and race-prone: a concurrent insert
// could still slip between a pre-check and this Insert call).
func CreateCustomer(ctx context.Context, uow domain.UnitOfWork, input CreateCustomerInput) (domain.Customer, error) {
	customer, err := domain.NewCustomer(domain.NewID(), input.TenantID, input.Name, input.Email, input.Phone)
	if err != nil {
		return domain.Customer{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		return r.Customers().Insert(ctx, &customer)
	})
	if err != nil {
		return domain.Customer{}, err
	}

	return customer, nil
}
