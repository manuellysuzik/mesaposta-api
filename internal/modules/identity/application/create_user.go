package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Audit action/entity identifiers this use case records. Constants rather
// than inline literals for the same reason
// internal/modules/crm/application.CreditLoyaltyPoints's own audit
// constants are: the value of an audit trail depends on these identifiers
// being STABLE across time.
const (
	auditActionUserCreated = "identity.user_created"
	auditEntityUser        = "user"
)

// CreateUserInput carries everything CreateUser needs to invite a
// brand-new staff User into TenantID.
type CreateUserInput struct {
	TenantID tenant.TenantID
	Email    string
	Password string
	Role     domain.Role
}

// CreateUser is the application-layer use case a gestor uses to invite a
// new staff member (POST /v1/users -- see
// ../infrastructure/http/user_handlers.go, mounted gestor-only at the
// composition root, exactly like internal/modules/ordering's own
// role-differentiated routes). There is no public self-registration
// endpoint anywhere in this module -- see this module's task notes.
//
// input.Password is hashed here, via internal/platform/auth/password.Hash,
// BEFORE domain.NewUser is called: domain/ must never see a plaintext
// password or know Argon2id exists (architecture rule R1).
//
// actorID identifies the gestor who requested the invite, for the audit
// trail -- sourced by the HTTP layer from internal/platform/auth.ActorIDFrom,
// mirroring internal/modules/crm/application.CreditLoyaltyPoints's own
// actorID parameter exactly. See docs/adr/0006-audit-log.md.
func CreateUser(ctx context.Context, uow domain.UnitOfWork, input CreateUserInput, actorID string) (domain.User, error) {
	hash, err := password.Hash(input.Password)
	if err != nil {
		return domain.User{}, err
	}

	user, err := domain.NewUser(domain.NewID(), input.TenantID, input.Email, hash, input.Role)
	if err != nil {
		return domain.User{}, err
	}

	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		if err := r.Users().Insert(ctx, &user); err != nil {
			return err
		}

		auditEntry, err := audit.NewEntry(
			user.TenantID(),
			actorID,
			auditActionUserCreated,
			auditEntityUser,
			user.ID().String(),
			user.CreatedAt(),
			nil,
			map[string]any{"email": user.Email(), "role": string(user.Role())},
		)
		if err != nil {
			return err
		}
		return r.Audit().Record(ctx, auditEntry)
	})
	if err != nil {
		return domain.User{}, err
	}

	return user, nil
}
