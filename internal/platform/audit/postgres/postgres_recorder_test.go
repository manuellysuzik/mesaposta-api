package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
)

func TestNullableActor(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		want    any
	}{
		{"empty actor id becomes SQL NULL", "", nil},
		{"non-empty actor id is passed through", "user-42", "user-42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nullableActor(tt.actorID); got != tt.want {
				t.Errorf("nullableActor(%q) = %v, want %v", tt.actorID, got, tt.want)
			}
		})
	}
}

func TestNullableJSON_NilSnapshot_IsSQLNull(t *testing.T) {
	got, err := nullableJSON(nil)
	if err != nil {
		t.Fatalf("nullableJSON(nil): unexpected error: %v", err)
	}
	// A nil []byte becomes SQL NULL. Marshaling nil directly would have
	// produced the four bytes "null" -- a present jsonb document, not an
	// absent column.
	if got != nil {
		t.Errorf("nullableJSON(nil) = %q, want nil (SQL NULL)", got)
	}
}

func TestNullableJSON_NonNilSnapshot_Marshals(t *testing.T) {
	got, err := nullableJSON(map[string]any{"status": "pendente", "version": 3})
	if err != nil {
		t.Fatalf("nullableJSON: unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshaling result %q: %v", got, err)
	}
	if decoded["status"] != "pendente" {
		t.Errorf("decoded[status] = %v, want %q", decoded["status"], "pendente")
	}
	if decoded["version"] != float64(3) {
		t.Errorf("decoded[version] = %v, want 3", decoded["version"])
	}
}

func TestNullableJSON_UnmarshalableSnapshot_ReturnsError(t *testing.T) {
	// A channel cannot be marshaled to JSON; WithScope must surface that
	// as an error rather than silently recording a broken entry.
	if _, err := nullableJSON(make(chan int)); err == nil {
		t.Fatal("nullableJSON(chan int): want an error, got nil")
	}
}

func TestRecorder_Record_EmptyEntries_IsNoOp(t *testing.T) {
	r := NewRecorder(nil)
	if err := r.Record(context.Background()); err != nil {
		t.Errorf("Record with zero entries: want nil error, got %v", err)
	}
}

func TestRecorder_Record_CancelledContext_ReturnsWithoutTouchingPool(t *testing.T) {
	r := NewRecorder(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entry := audit.Entry{Action: "purchase_order.received"}
	if err := r.Record(ctx, entry); err == nil {
		t.Fatal("Record: want an error for an already-cancelled context, got nil")
	}
}

func TestWithScope_EmptyEntries_IsNoOp(t *testing.T) {
	if err := WithScope(context.Background(), nil); err != nil {
		t.Errorf("WithScope with zero entries: want nil error, got %v", err)
	}
}
