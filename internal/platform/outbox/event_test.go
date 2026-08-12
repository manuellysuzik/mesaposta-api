package outbox_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
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

func TestNewDomainEvent_ValidInput_Succeeds(t *testing.T) {
	// Arrange
	tenantID := mustTenantID(t)
	occurredAt := time.Now()

	// Act
	evt, err := outbox.NewDomainEvent(tenantID, "bet.placed", "bet-123", occurredAt, map[string]any{"amount": 10})

	// Assert
	if err != nil {
		t.Fatalf("NewDomainEvent: unexpected error: %v", err)
	}
	if evt.TenantID != tenantID {
		t.Errorf("evt.TenantID = %v, want %v", evt.TenantID, tenantID)
	}
	if evt.Name != "bet.placed" {
		t.Errorf("evt.Name = %q, want %q", evt.Name, "bet.placed")
	}
	if evt.AggregateID != "bet-123" {
		t.Errorf("evt.AggregateID = %q, want %q", evt.AggregateID, "bet-123")
	}
	if !evt.OccurredAt.Equal(occurredAt) {
		t.Errorf("evt.OccurredAt = %v, want %v", evt.OccurredAt, occurredAt)
	}
}

func TestNewDomainEvent_InvalidInput_FailsAtConstruction(t *testing.T) {
	// Arrange
	validTenant := mustTenantID(t)
	validName := "bet.placed"
	validAggregateID := "bet-123"
	validOccurredAt := time.Now()

	tests := []struct {
		name        string
		tenantID    tenant.TenantID
		eventName   string
		aggregateID string
		occurredAt  time.Time
	}{
		{
			name:        "zero tenant id",
			tenantID:    tenant.TenantID{},
			eventName:   validName,
			aggregateID: validAggregateID,
			occurredAt:  validOccurredAt,
		},
		{
			name:        "empty event name",
			tenantID:    validTenant,
			eventName:   "",
			aggregateID: validAggregateID,
			occurredAt:  validOccurredAt,
		},
		{
			name:        "empty aggregate id",
			tenantID:    validTenant,
			eventName:   validName,
			aggregateID: "",
			occurredAt:  validOccurredAt,
		},
		{
			name:        "zero occurred at",
			tenantID:    validTenant,
			eventName:   validName,
			aggregateID: validAggregateID,
			occurredAt:  time.Time{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Act
			evt, err := outbox.NewDomainEvent(tt.tenantID, tt.eventName, tt.aggregateID, tt.occurredAt, nil)

			// Assert -- validation must happen at construction time, not
			// later at use time: NewDomainEvent itself must reject the
			// invalid input and never hand back a usable DomainEvent.
			if err == nil {
				t.Fatalf("NewDomainEvent(%+v) returned nil error, want an error", tt)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("NewDomainEvent(%+v) error kind = %v, want %v", tt, errs.KindOf(err), errs.Invalid)
			}
			if evt != (outbox.DomainEvent{}) {
				t.Errorf("NewDomainEvent(%+v) returned non-zero DomainEvent %+v on error", tt, evt)
			}
		})
	}
}
