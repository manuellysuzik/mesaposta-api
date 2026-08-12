package outbox_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// compile-time assertion: *MemoryPublisher must satisfy DomainEventPublisher.
var _ outbox.DomainEventPublisher = (*outbox.MemoryPublisher)(nil)

func mustDomainEvent(t *testing.T, name, aggregateID string) outbox.DomainEvent {
	t.Helper()
	evt, err := outbox.NewDomainEvent(mustTenantID(t), name, aggregateID, time.Now(), nil)
	if err != nil {
		t.Fatalf("NewDomainEvent: %v", err)
	}
	return evt
}

func TestMemoryPublisher_Publish_AppendsEvents(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()
	evt1 := mustDomainEvent(t, "bet.placed", "bet-1")
	evt2 := mustDomainEvent(t, "bet.settled", "bet-1")

	// Act
	err := p.Publish(context.Background(), evt1, evt2)

	// Assert
	if err != nil {
		t.Fatalf("Publish: unexpected error: %v", err)
	}
	got := p.Events()
	if len(got) != 2 {
		t.Fatalf("len(Events()) = %d, want 2", len(got))
	}
	if got[0] != evt1 || got[1] != evt2 {
		t.Errorf("Events() = %+v, want [%+v %+v]", got, evt1, evt2)
	}
}

func TestMemoryPublisher_Publish_NoEvents_IsNoop(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()

	// Act
	err := p.Publish(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Publish(no events): unexpected error: %v", err)
	}
	if got := p.Events(); len(got) != 0 {
		t.Errorf("Events() = %+v, want empty", got)
	}
}

func TestMemoryPublisher_Publish_MultipleCalls_Accumulate(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()
	evt1 := mustDomainEvent(t, "bet.placed", "bet-1")
	evt2 := mustDomainEvent(t, "bet.settled", "bet-1")

	// Act
	if err := p.Publish(context.Background(), evt1); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := p.Publish(context.Background(), evt2); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Assert
	if got := p.Events(); len(got) != 2 {
		t.Fatalf("len(Events()) = %d, want 2", len(got))
	}
}

func TestMemoryPublisher_Publish_CanceledContext_ReturnsError(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evt := mustDomainEvent(t, "bet.placed", "bet-1")

	// Act
	err := p.Publish(ctx, evt)

	// Assert
	if err == nil {
		t.Fatal("Publish(canceled ctx): expected an error, got nil")
	}
	if got := p.Events(); len(got) != 0 {
		t.Errorf("Events() = %+v, want empty after publishing with a canceled context", got)
	}
}

func TestMemoryPublisher_Events_ReturnsIndependentCopy(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()
	evt := mustDomainEvent(t, "bet.placed", "bet-1")
	if err := p.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Act -- mutate the slice returned by Events().
	got := p.Events()
	got[0] = outbox.DomainEvent{}

	// Assert -- internal state must be unaffected (defensive copy, per the
	// project's immutability rule).
	again := p.Events()
	if again[0] != evt {
		t.Errorf("internal state mutated via Events() return value: got %+v, want %+v", again[0], evt)
	}
}

func TestMemoryPublisher_Publish_ConcurrentCalls_AreRaceFree(t *testing.T) {
	// Arrange
	p := outbox.NewMemoryPublisher()
	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Act
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			evt := mustDomainEvent(t, "bet.placed", "bet-concurrent")
			_ = p.Publish(context.Background(), evt)
		}(i)
	}
	wg.Wait()

	// Assert
	if got := p.Events(); len(got) != goroutines {
		t.Errorf("len(Events()) = %d, want %d", len(got), goroutines)
	}
}
