package postgres

import (
	"context"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

// TransactionRepository is the Postgres-backed implementation of
// domain.TransactionRepository, backed by the (append-only) transactions
// table (see internal/platform/db/migrations/0004_operations.sql). It
// exposes only Insert, matching domain.TransactionRepository's own
// append-only shape and domain.Transaction's own doc comment ("immutable
// once created ... this phase exposes no setter") -- the underlying
// table's GRANT (SELECT, INSERT only) enforces that immutability at the
// database layer, and its transactions_reference_compras_unique_idx
// partial unique index is this module's acceptance criterion 2 enforced as
// a physical backstop: a second "compras" Transaction for the same
// reference_id is rejected by Postgres itself, independent of anything
// this Go type does or fails to do.
type TransactionRepository struct {
	scope *db.Scope
}

// compile-time check: *TransactionRepository must satisfy domain.TransactionRepository.
var _ domain.TransactionRepository = (*TransactionRepository)(nil)

// NewTransactionRepository builds a TransactionRepository bound to scope --
// see StockItemRepository's NewStockItemRepository doc comment for why
// this constructor exists alongside UnitOfWork.Do/Repositories.
// Transactions().
func NewTransactionRepository(scope *db.Scope) *TransactionRepository {
	return &TransactionRepository{scope: scope}
}

// Insert persists a brand-new Transaction. A violation of
// transactions_reference_compras_unique_idx (a duplicate "compras"
// reference_id) surfaces here as a plain wrapped Postgres error -- this
// repository does not attempt to reclassify it into an *errs.Error, since
// no current caller in this module relies on Insert itself detecting that
// specific conflict (domain.ReceivePurchaseOrder's own
// PurchaseOrder.MarkAsReceived/UpdateWithVersion optimistic-concurrency
// check is what prevents two concurrent receipts of the SAME order from
// ever reaching two competing Transaction inserts in the first place -- see
// purchase_order_repository.go's UpdateWithVersion doc comment).
func (r *TransactionRepository) Insert(ctx context.Context, transaction domain.Transaction) error {
	var referenceIDArg any
	if id := transaction.ReferenceID(); id != nil {
		referenceIDArg = id.String()
	}

	_, err := r.scope.Exec(ctx, `
		INSERT INTO transactions (id, tenant_id, type, category, description, amount, date, reference_id)
		VALUES ($1, $2, $3, $4, $5, $6::numeric / 100, $7, $8)
	`,
		transaction.ID().String(), r.scope.TenantID().String(), string(transaction.TransactionType()),
		string(transaction.Category()), transaction.Description(), transaction.Amount().Cents(),
		transaction.Date(), referenceIDArg,
	)
	if err != nil {
		return fmt.Errorf("operations/postgres: inserting transaction: %w", err)
	}
	return nil
}
