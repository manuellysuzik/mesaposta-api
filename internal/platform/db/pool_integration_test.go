//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// TestPool_Ping_CancelledContext_FailsFast confirms Ping honors an
// already-cancelled context instead of blocking on a real acquire attempt.
func TestPool_Ping_CancelledContext_FailsFast(t *testing.T) {
	adminURL, _ := TestPostgres(t)

	pool, err := Open(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err = pool.Ping(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Ping: want error for an already-cancelled context, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Ping with a cancelled context took %v, want it to fail fast", elapsed)
	}
}

func TestPool_Ping_Succeeds(t *testing.T) {
	adminURL, _ := TestPostgres(t)

	pool, err := Open(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: unexpected error: %v", err)
	}
}

func TestPool_Close_IsIdempotent_AgainstARealPool(t *testing.T) {
	adminURL, _ := TestPostgres(t)

	pool, err := Open(context.Background(), adminURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	pool.Close()
	pool.Close() // must not panic
}

func TestPool_Stat_RespectsConfiguredMaxConns(t *testing.T) {
	adminURL, _ := TestPostgres(t)

	const maxConns = int32(3)
	pool, err := Open(context.Background(), adminURL, WithMaxConns(maxConns))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if got := pool.Stat().MaxConns(); got != maxConns {
		t.Errorf("Stat().MaxConns() = %d, want %d", got, maxConns)
	}
}
