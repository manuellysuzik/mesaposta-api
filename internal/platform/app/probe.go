package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	outboxpg "github.com/manuellysuzik/mesaposta-api/internal/platform/outbox/postgres"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// probeAggregateIDBytes is the amount of random entropy (in bytes) used to
// build a probe event's aggregate id -- enough to make two concurrent test
// runs' aggregate ids collide only astronomically rarely, without needing a
// real UUID library (see internal/platform/tenant's own doc comment on
// avoiding third-party UUID dependencies where a hand-rolled random id
// suffices).
const probeAggregateIDBytes = 16

// probeEventResponse is the JSON payload both probe endpoints return.
type probeEventResponse struct {
	AggregateID string `json:"aggregate_id"`
	EventName   string `json:"event_name"`
	TenantID    string `json:"tenant_id"`
}

// newProbeAggregateID generates a random, unguessable aggregate id for a
// single probe event; a caller correlates a POST's response with a
// subsequent GET via this value.
func newProbeAggregateID() (string, error) {
	buf := make([]byte, probeAggregateIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// createProbeEventHandler builds the handler for POST /v1/_probe/events: it
// reads the authenticated tenant from the request context (tenant.From),
// records a single domain event for it via the Postgres outbox publisher
// from Bloco E (internal/platform/outbox/postgres), and returns the
// generated aggregate id a caller can later GET back via
// getProbeEventHandler.
//
// This handler -- together with getProbeEventHandler -- is the whole point
// of internal/platform/httpx's own end-to-end test (T19): it proves JWT
// authentication -> tenant context -> a real, row-level-security-enforced
// Postgres write/read are wired together correctly, without needing any
// real domain module to exist yet.
func createProbeEventHandler(pool *db.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		aggregateID, err := newProbeAggregateID()
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		event, err := outbox.NewDomainEvent(tenantID, "probe.ping", aggregateID, time.Now().UTC(),
			map[string]any{"path": r.URL.Path})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := outboxpg.NewPublisher(pool).Publish(r.Context(), event); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, probeEventResponse{
			AggregateID: aggregateID,
			EventName:   event.Name,
			TenantID:    tenantID.String(),
		}, nil)
	}
}

// probeEventNotFoundErr builds the exact same NotFoundError (same code,
// same public message) for both "no probe event with this aggregate id was
// ever created" and "one was created, but by a different tenant" -- the two
// cases must be indistinguishable to the caller (see
// httpx.NotFoundError's doc comment), and Postgres row-level security
// already guarantees the underlying SELECT itself cannot even see the
// second case's row in the first place.
func probeEventNotFoundErr(cause error) error {
	return httpx.NotFoundError("probe_event_not_found", "probe event not found", cause)
}

// selectProbeEventSQL looks up the most recent probe event for a given
// aggregate id. It relies entirely on the tenant-scoped db.Scope it runs
// through (see getProbeEventHandler) for tenant isolation: no tenant_id
// predicate appears here because Postgres row-level security -- via the
// app.tenant_id session-local setting db.Scope populates -- already
// restricts every row this query can possibly see to the caller's own
// tenant.
const selectProbeEventSQL = `
	SELECT event_name FROM domain_events
	WHERE aggregate_id = $1
	ORDER BY occurred_at DESC
	LIMIT 1
`

// getProbeEventHandler builds the handler for GET
// /v1/_probe/events/{aggregateID}: it opens a tenant-scoped db.Scope --
// which both requires ctx to already carry an authenticated tenant (see
// internal/platform/db's package doc comment) and enforces Postgres
// row-level security for the lifetime of the query -- and looks up the
// most recent probe event with the given aggregate id.
func getProbeEventHandler(pool *db.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aggregateID := chi.URLParam(r, "aggregateID")

		scope, err := pool.Scope(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		defer func() { _ = scope.Rollback(r.Context()) }()

		var eventName string
		err = scope.QueryRow(r.Context(), selectProbeEventSQL, aggregateID).Scan(&eventName)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.RespondError(w, r, probeEventNotFoundErr(err))
			return
		}
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := scope.Commit(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, probeEventResponse{
			AggregateID: aggregateID,
			EventName:   eventName,
			TenantID:    scope.TenantID().String(),
		}, nil)
	}
}
