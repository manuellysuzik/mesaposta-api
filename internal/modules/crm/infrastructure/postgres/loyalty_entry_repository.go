package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// LoyaltyEntryRepository is the Postgres-backed implementation of
// domain.LoyaltyEntryRepository, backed by the (append-only)
// loyalty_entries table (see
// internal/platform/db/migrations/0006_crm.sql). There is deliberately no
// FindByID/FindByCustomerID method: this phase has no use case that reads
// the ledger back (see this module's final report for that deliberately
// deferred scope decision), so LoyaltyEntryRepository only exposes what
// domain.LoyaltyEntryRepository declares -- Insert.
type LoyaltyEntryRepository struct {
	scope *db.Scope
}

// compile-time check: *LoyaltyEntryRepository must satisfy domain.LoyaltyEntryRepository.
var _ domain.LoyaltyEntryRepository = (*LoyaltyEntryRepository)(nil)

// NewLoyaltyEntryRepository builds a LoyaltyEntryRepository bound to scope.
// Most callers reach a LoyaltyEntryRepository via
// UnitOfWork.Do/Repositories.LoyaltyEntries() instead; this constructor
// exists so this package's own integration test suite (and any other
// caller that already holds a *db.Scope) can exercise it directly.
func NewLoyaltyEntryRepository(scope *db.Scope) *LoyaltyEntryRepository {
	return &LoyaltyEntryRepository{scope: scope}
}

// Insert persists a brand-new LoyaltyEntry.
func (r *LoyaltyEntryRepository) Insert(ctx context.Context, entry domain.LoyaltyEntry) error {
	_, err := r.scope.Exec(ctx, `
		INSERT INTO loyalty_entries (id, tenant_id, customer_id, type, points, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		entry.ID().String(), r.scope.TenantID().String(), entry.CustomerID().String(),
		string(entry.EntryType()), entry.Points().Points(), entry.Reason(), entry.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("crm/postgres: inserting loyalty entry: %w", err)
	}
	return nil
}
