package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// mutateTable loads the Table identified by id, applies mutate to it, and
// -- only if mutate actually changed the aggregate's Version (see
// domain.Table.Activate/Deactivate's own doc comment on why those methods
// are deliberately idempotent no-ops rather than always bumping Version) --
// persists the result via UpdateWithVersion, enforcing optimistic
// concurrency against the version the table was loaded at. Skipping the
// UpdateWithVersion call entirely when nothing changed is what makes
// ActivateTable/DeactivateTable genuinely idempotent end-to-end, not just
// at the in-memory aggregate level: a redundant call issues NO write at
// all, not a no-op write that would still (harmlessly, but wastefully)
// advance Version.
func mutateTable(ctx context.Context, uow domain.UnitOfWork, id domain.ID, mutate func(*domain.Table)) (domain.Table, error) {
	var table domain.Table
	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		found, err := r.Tables().FindByID(ctx, id)
		if err != nil {
			return err
		}
		expectedVersion := found.Version()

		mutate(found)

		if found.Version() != expectedVersion {
			if err := r.Tables().UpdateWithVersion(ctx, found, expectedVersion); err != nil {
				return err
			}
		}
		table = *found
		return nil
	})
	if err != nil {
		return domain.Table{}, err
	}
	return table, nil
}

// ActivateTable marks the Table identified by id active -- see
// domain.Table.Activate's own doc comment for why this is idempotent.
func ActivateTable(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Table, error) {
	return mutateTable(ctx, uow, id, (*domain.Table).Activate)
}

// DeactivateTable marks the Table identified by id inactive -- see
// domain.Table.Deactivate's own doc comment for why this is idempotent.
func DeactivateTable(ctx context.Context, uow domain.UnitOfWork, id domain.ID) (domain.Table, error) {
	return mutateTable(ctx, uow, id, (*domain.Table).Deactivate)
}
