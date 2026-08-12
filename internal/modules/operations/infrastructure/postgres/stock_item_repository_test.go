package postgres

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/operations/domain"
)

// TestStockItemRepository_FindManyByIDs_EmptyIDs_NeverTouchesTheScope
// pins down FindManyByIDs' documented early return for an empty ids slice:
// it must return an empty, non-nil map WITHOUT calling r.scope.Query at
// all -- proved here by constructing a StockItemRepository with a nil
// *db.Scope. If this method ever stopped short-circuiting before touching
// scope, this test would panic (a nil pointer dereference) instead of
// merely failing an assertion.
func TestStockItemRepository_FindManyByIDs_EmptyIDs_NeverTouchesTheScope(t *testing.T) {
	repo := NewStockItemRepository(nil)

	got, err := repo.FindManyByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("FindManyByIDs(nil ids): unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("FindManyByIDs(nil ids): want a non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("FindManyByIDs(nil ids): len = %d, want 0", len(got))
	}

	got2, err := repo.FindManyByIDs(context.Background(), []domain.ID{})
	if err != nil {
		t.Fatalf("FindManyByIDs(empty ids): unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Errorf("FindManyByIDs(empty ids): len = %d, want 0", len(got2))
	}
}
