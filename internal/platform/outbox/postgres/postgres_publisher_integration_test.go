//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	outboxpg "github.com/manuellysuzik/mesaposta-api/internal/platform/outbox/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

func mustEvent(t *testing.T, tenantID tenant.TenantID, name, aggregateID string, occurredAt time.Time, payload any) outbox.DomainEvent {
	t.Helper()
	ev, err := outbox.NewDomainEvent(tenantID, name, aggregateID, occurredAt, payload)
	if err != nil {
		t.Fatalf("outbox.NewDomainEvent: %v", err)
	}
	return ev
}

func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	_, appURL := db.TestPostgres(t)
	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPublisher_Publish_UsesScopeTenantID_NotEventTenantID(t *testing.T) {
	pool := openTestPool(t)
	publisher := outboxpg.NewPublisher(pool)

	tenantA := mustTenantID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantB := mustTenantID(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	// A "forged" event: constructed with tenant B's id, but published
	// through tenant A's context/scope.
	forged := mustEvent(t, tenantB, "bet.placed", "agg-1", time.Now().UTC(), map[string]any{"amount": 10})

	ctxA := tenant.With(context.Background(), tenantA)
	err := publisher.Publish(ctxA, forged)
	if err == nil {
		t.Fatal("Publish: want an error for an event whose TenantID does not match the scope's tenant, got nil")
	}

	// Nothing must have been persisted -- not even under tenant B, and
	// certainly not silently rewritten to tenant A.
	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope (tenant A): %v", err)
	}
	defer scopeA.Rollback(ctxA)
	var countA int
	if err := scopeA.QueryRow(ctxA, "SELECT count(*) FROM domain_events").Scan(&countA); err != nil {
		t.Fatalf("counting tenant A's events: %v", err)
	}
	if countA != 0 {
		t.Fatalf("tenant A has %d event(s) after a rejected forged Publish, want 0", countA)
	}

	ctxB := tenant.With(context.Background(), tenantB)
	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (tenant B): %v", err)
	}
	defer scopeB.Rollback(ctxB)
	var countB int
	if err := scopeB.QueryRow(ctxB, "SELECT count(*) FROM domain_events").Scan(&countB); err != nil {
		t.Fatalf("counting tenant B's events: %v", err)
	}
	if countB != 0 {
		t.Fatalf("tenant B has %d event(s) after a rejected forged Publish, want 0", countB)
	}
}

func TestPublisher_Publish_BatchFailure_PersistsNothing(t *testing.T) {
	pool := openTestPool(t)
	publisher := outboxpg.NewPublisher(pool)

	tenantA := mustTenantID(t, "cccccccc-cccc-cccc-cccc-cccccccccccc")
	tenantB := mustTenantID(t, "dddddddd-dddd-dddd-dddd-dddddddddddd")
	ctxA := tenant.With(context.Background(), tenantA)

	now := time.Now().UTC()
	good1 := mustEvent(t, tenantA, "bet.placed", "agg-1", now, nil)
	good2 := mustEvent(t, tenantA, "bet.settled", "agg-1", now.Add(time.Second), nil)
	// 3rd item: tenant mismatch forces a mid-batch failure.
	bad := mustEvent(t, tenantB, "bet.voided", "agg-1", now.Add(2*time.Second), nil)

	err := publisher.Publish(ctxA, good1, good2, bad)
	if err == nil {
		t.Fatal("Publish: want an error when an item in the batch fails, got nil")
	}

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scopeA.Rollback(ctxA)
	var count int
	if err := scopeA.QueryRow(ctxA, "SELECT count(*) FROM domain_events").Scan(&count); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d event(s) persisted after a batch containing a failing item, want 0 (all-or-nothing)", count)
	}
}

func TestPublisher_Publish_TenantIsolation(t *testing.T) {
	pool := openTestPool(t)
	publisher := outboxpg.NewPublisher(pool)

	tenantA := mustTenantID(t, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	tenantB := mustTenantID(t, "ffffffff-ffff-ffff-ffff-ffffffffffff")

	ctxA := tenant.With(context.Background(), tenantA)
	ev := mustEvent(t, tenantA, "bet.placed", "agg-1", time.Now().UTC(), nil)
	if err := publisher.Publish(ctxA, ev); err != nil {
		t.Fatalf("Publish (tenant A): %v", err)
	}

	ctxB := tenant.With(context.Background(), tenantB)
	scopeB, err := pool.Scope(ctxB)
	if err != nil {
		t.Fatalf("Scope (tenant B): %v", err)
	}
	defer scopeB.Rollback(ctxB)

	var count int
	if err := scopeB.QueryRow(ctxB, "SELECT count(*) FROM domain_events").Scan(&count); err != nil {
		t.Fatalf("counting events visible to tenant B: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant B sees %d event(s) published by tenant A, want 0", count)
	}
}

func TestPublisher_Publish_StableOrdering(t *testing.T) {
	pool := openTestPool(t)
	publisher := outboxpg.NewPublisher(pool)

	tenantA := mustTenantID(t, "01234567-0123-0123-0123-012345678901")
	ctxA := tenant.With(context.Background(), tenantA)

	base := time.Now().UTC().Truncate(time.Millisecond)
	events := []outbox.DomainEvent{
		mustEvent(t, tenantA, "step.three", "agg-1", base.Add(2*time.Second), nil),
		mustEvent(t, tenantA, "step.one", "agg-1", base, nil),
		mustEvent(t, tenantA, "step.two", "agg-1", base.Add(time.Second), nil),
	}
	for _, ev := range events {
		if err := publisher.Publish(ctxA, ev); err != nil {
			t.Fatalf("Publish %q: %v", ev.Name, err)
		}
	}

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scopeA.Rollback(ctxA)

	rows, err := scopeA.Query(ctxA, "SELECT event_name FROM domain_events ORDER BY tenant_id, occurred_at, id")
	if err != nil {
		t.Fatalf("querying ordered events: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning event_name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}

	want := []string{"step.one", "step.two", "step.three"}
	if len(names) != len(want) {
		t.Fatalf("got %d event names, want %d: %v", len(names), len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q (order: %v)", i, names[i], want[i], names)
		}
	}
}

func TestPublisher_Publish_PayloadRoundTrips(t *testing.T) {
	pool := openTestPool(t)
	publisher := outboxpg.NewPublisher(pool)

	tenantA := mustTenantID(t, "89abcdef-89ab-89ab-89ab-89abcdef0123")
	ctxA := tenant.With(context.Background(), tenantA)

	payload := map[string]any{"amount": float64(42), "currency": "BRL"}
	ev := mustEvent(t, tenantA, "bet.placed", "agg-1", time.Now().UTC(), payload)

	if err := publisher.Publish(ctxA, ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	scopeA, err := pool.Scope(ctxA)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	defer scopeA.Rollback(ctxA)

	var raw []byte
	if err := scopeA.QueryRow(ctxA, "SELECT payload FROM domain_events WHERE event_name = 'bet.placed'").Scan(&raw); err != nil {
		t.Fatalf("reading payload: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshaling stored payload: %v", err)
	}
	if got["currency"] != "BRL" {
		t.Errorf("payload[currency] = %v, want %q", got["currency"], "BRL")
	}
	if got["amount"] != float64(42) {
		t.Errorf("payload[amount] = %v, want 42", got["amount"])
	}
}
