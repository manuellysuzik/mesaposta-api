package postgres

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

func TestAggregateTypeOf(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      string
	}{
		{"dot-namespaced name uses prefix", "bet.placed", "bet"},
		{"multiple dots uses only first segment", "bet.settlement.completed", "bet"},
		{"no dot falls back to the full name", "betplaced", "betplaced"},
		{"empty name falls back to empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := outbox.DomainEvent{Name: tt.eventName}
			if got := aggregateTypeOf(ev); got != tt.want {
				t.Errorf("aggregateTypeOf(%+v) = %q, want %q", ev, got, tt.want)
			}
		})
	}
}

func TestPublisher_Publish_EmptyEvents_IsNoOp(t *testing.T) {
	p := NewPublisher(nil)
	if err := p.Publish(context.Background()); err != nil {
		t.Errorf("Publish with zero events: want nil error, got %v", err)
	}
}

func TestPublisher_Publish_CancelledContext_ReturnsWithoutTouchingPool(t *testing.T) {
	p := NewPublisher(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ev := outbox.DomainEvent{Name: "bet.placed"}
	err := p.Publish(ctx, ev)
	if err == nil {
		t.Fatal("Publish: want an error for an already-cancelled context, got nil")
	}
}

func TestWithScope_EmptyEvents_IsNoOp(t *testing.T) {
	if err := WithScope(context.Background(), nil); err != nil {
		t.Errorf("WithScope with zero events: want nil error, got %v", err)
	}
}
