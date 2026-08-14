package audit_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const validUUID = "11111111-2222-3333-4444-555555555555"

func mustTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(validUUID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", validUUID, err)
	}
	return id
}

func TestNewEntry_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	tenantID := mustTenantID(t)
	occurredAt := time.Now()
	before := map[string]any{"status": "pendente"}
	after := map[string]any{"status": "recebido"}

	// Act
	entry, err := audit.NewEntry(tenantID, "user-42", "purchase_order.received", "purchase_order", "po-123", occurredAt, before, after)

	// Assert
	if err != nil {
		t.Fatalf("NewEntry: unexpected error: %v", err)
	}
	if entry.TenantID != tenantID {
		t.Errorf("entry.TenantID = %v, want %v", entry.TenantID, tenantID)
	}
	if entry.ActorID != "user-42" {
		t.Errorf("entry.ActorID = %q, want %q", entry.ActorID, "user-42")
	}
	if entry.Action != "purchase_order.received" {
		t.Errorf("entry.Action = %q, want %q", entry.Action, "purchase_order.received")
	}
	if entry.EntityType != "purchase_order" {
		t.Errorf("entry.EntityType = %q, want %q", entry.EntityType, "purchase_order")
	}
	if entry.EntityID != "po-123" {
		t.Errorf("entry.EntityID = %q, want %q", entry.EntityID, "po-123")
	}
	if !entry.OccurredAt.Equal(occurredAt) {
		t.Errorf("entry.OccurredAt = %v, want %v", entry.OccurredAt, occurredAt)
	}
	beforeMap, ok := entry.Before.(map[string]any)
	if !ok || beforeMap["status"] != "pendente" {
		t.Errorf("entry.Before = %v, want %v", entry.Before, before)
	}
	afterMap, ok := entry.After.(map[string]any)
	if !ok || afterMap["status"] != "recebido" {
		t.Errorf("entry.After = %v, want %v", entry.After, after)
	}
}

// TestNewEntry_OptionalFields_AreAccepted pins down the deliberate design
// decision documented on Entry.ActorID/Before/After: an action with no
// identifiable human actor (a system process), no prior state (a
// creation), and no resulting state (a deletion) must all still be
// recordable. Rejecting them would push callers toward inventing
// placeholder values, which is strictly worse for a trail whose entire
// value is that it is trustworthy.
func TestNewEntry_OptionalFields_AreAccepted(t *testing.T) {
	// Arrange
	tenantID := mustTenantID(t)

	// Act -- empty actor id, nil before, nil after.
	entry, err := audit.NewEntry(tenantID, "", "stock_item.reconciled", "stock_item", "si-1", time.Now(), nil, nil)

	// Assert
	if err != nil {
		t.Fatalf("NewEntry with empty actor and nil before/after: unexpected error: %v", err)
	}
	if entry.ActorID != "" {
		t.Errorf("entry.ActorID = %q, want %q", entry.ActorID, "")
	}
	if entry.Before != nil {
		t.Errorf("entry.Before = %v, want nil", entry.Before)
	}
	if entry.After != nil {
		t.Errorf("entry.After = %v, want nil", entry.After)
	}
}

func TestNewEntry_InvalidInput_FailsAtConstruction(t *testing.T) {
	// Arrange
	validTenant := mustTenantID(t)
	const (
		validAction     = "purchase_order.received"
		validEntityType = "purchase_order"
		validEntityID   = "po-123"
	)
	validOccurredAt := time.Now()

	tests := []struct {
		name       string
		tenantID   tenant.TenantID
		action     string
		entityType string
		entityID   string
		occurredAt time.Time
	}{
		{
			name:       "zero tenant id",
			tenantID:   tenant.TenantID{},
			action:     validAction,
			entityType: validEntityType,
			entityID:   validEntityID,
			occurredAt: validOccurredAt,
		},
		{
			name:       "empty action",
			tenantID:   validTenant,
			action:     "",
			entityType: validEntityType,
			entityID:   validEntityID,
			occurredAt: validOccurredAt,
		},
		{
			name:       "empty entity type",
			tenantID:   validTenant,
			action:     validAction,
			entityType: "",
			entityID:   validEntityID,
			occurredAt: validOccurredAt,
		},
		{
			name:       "empty entity id",
			tenantID:   validTenant,
			action:     validAction,
			entityType: validEntityType,
			entityID:   "",
			occurredAt: validOccurredAt,
		},
		{
			name:       "zero occurred at",
			tenantID:   validTenant,
			action:     validAction,
			entityType: validEntityType,
			entityID:   validEntityID,
			occurredAt: time.Time{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Act
			entry, err := audit.NewEntry(tt.tenantID, "actor-1", tt.action, tt.entityType, tt.entityID, tt.occurredAt, nil, nil)

			// Assert -- validation must happen at construction time, not
			// later at record time: NewEntry itself must reject the
			// invalid input and never hand back a usable Entry.
			if err == nil {
				t.Fatalf("NewEntry(%+v) returned nil error, want an error", tt)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("NewEntry(%+v) error kind = %v, want %v", tt, errs.KindOf(err), errs.Invalid)
			}
			if entry.TenantID != (tenant.TenantID{}) || entry.Action != "" || entry.EntityType != "" ||
				entry.EntityID != "" || entry.ActorID != "" || !entry.OccurredAt.IsZero() ||
				entry.Before != nil || entry.After != nil {
				t.Errorf("NewEntry(%+v) returned non-zero Entry %+v on error", tt, entry)
			}
		})
	}
}
