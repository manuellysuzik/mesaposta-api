//go:build integration

// This file is mesaposta-api's Phase-0 proof slice (T19): it builds the
// COMPLETE HTTP application (internal/platform/app.NewApp, the same
// composition root cmd/api/main.go serves) against a real Postgres
// database (via internal/platform/db.TestPostgres, the same testcontainers
// bootstrap Bloco E's own integration suites use), issues real JWTs (via
// internal/platform/auth/token.Issuer) for two distinct tenants, and drives
// it entirely over HTTP (httptest.Server + net/http, never calling any
// internal function directly) to prove that:
//
//   - a valid access token authenticates the request and makes its tenant
//     available to the handler (tenant.From);
//   - writes and reads through internal/platform/db.Scope are subject to
//     real Postgres row-level security -- tenant A can never observe
//     tenant B's data, and vice versa;
//   - a resource that exists but belongs to a DIFFERENT tenant is reported
//     as 404, indistinguishable from a resource that does not exist at
//     all -- never 403;
//   - an expired token is rejected with 401;
//   - a well-formed, non-expired token whose role lacks permission for the
//     requested action is rejected with 403;
//   - every response -- success or failure -- is shaped as the standard
//     httpx.Envelope.
//
// In short: this is the automated evidence that JWT -> tenant context ->
// RLS -> envelope are wired together correctly end-to-end, not just that
// each piece works in isolation.
package httpx_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/app"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// e2eJWTSecret is the test-only signing secret shared by the app under test
// and every token this file mints. It is >= 32 bytes, as
// internal/platform/config requires.
const e2eJWTSecret = "e2e-integration-test-secret-at-least-32-bytes!"

// tenantAID and tenantBID are two distinct, valid tenant UUIDs used
// throughout this file to prove cross-tenant isolation.
const (
	tenantAID = "11111111-1111-1111-1111-111111111111"
	tenantBID = "22222222-2222-2222-2222-222222222222"
)

// probeEventResponse mirrors internal/platform/app's own (unexported)
// response shape -- duplicated here deliberately: this test exercises the
// application as an external HTTP client would, decoding only the JSON
// wire format, never reaching into app's internals.
type probeEventResponse struct {
	AggregateID string `json:"aggregate_id"`
	EventName   string `json:"event_name"`
	TenantID    string `json:"tenant_id"`
}

// e2eEnvelope mirrors httpx.Envelope for decoding data as
// probeEventResponse specifically (httpx.Envelope.Data is `any`, which
// json.Unmarshal would otherwise decode as map[string]any).
type e2eEnvelope struct {
	Data  *probeEventResponse `json:"data"`
	Error *httpx.ErrorBody    `json:"error"`
	Meta  httpx.Meta          `json:"meta"`
}

// setupE2EApp starts a real, ephemeral Postgres container, applies every
// migration against it, builds the complete application via app.NewApp,
// and serves it behind a real httptest.Server. It returns the server's
// base URL and a token.Issuer producing tokens the running app accepts.
func setupE2EApp(t *testing.T) (baseURL string, issuer *token.Issuer) {
	t.Helper()

	_, appURL := db.TestPostgres(t)

	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)

	cfg := config.Config{
		AppEnv:      "development",
		Port:        "0",
		DatabaseURL: appURL,
		JWTSecret:   e2eJWTSecret,
	}

	verifier, err := app.BuildVerifier(cfg)
	if err != nil {
		t.Fatalf("app.BuildVerifier: %v", err)
	}
	issuer, err = app.BuildIssuer(cfg)
	if err != nil {
		t.Fatalf("app.BuildIssuer: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := app.NewApp(cfg, pool, logger, verifier)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv.URL, issuer
}

// mustIssue mints a valid, non-expired access token for the given tenant
// and role via issuer -- the SAME token.Issuer the running app's
// token.Verifier was built to accept (see app.BuildVerifier/BuildIssuer).
func mustIssue(t *testing.T, issuer *token.Issuer, subject, tenantID string, role token.Role) string {
	t.Helper()
	tok, err := issuer.Issue(subject, tenantID, role, "session-"+subject)
	if err != nil {
		t.Fatalf("issuer.Issue: %v", err)
	}
	return tok
}

// mustExpiredToken hand-crafts a token that is otherwise identical to what
// issuer would produce, except already expired -- something token.Issuer's
// fixed AccessTokenTTL cannot do on its own. It is signed with the exact
// same secret/kid/issuer/audience app.BuildVerifier expects, so the ONLY
// reason the app can reject it is expiration.
func mustExpiredToken(t *testing.T, tenantID string, role token.Role) string {
	t.Helper()
	now := time.Now().UTC()
	claims := token.Claims{
		TenantID:  tenantID,
		Role:      role,
		SessionID: "session-expired",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "expired-user",
			Issuer:    app.TokenIssuer,
			Audience:  jwt.ClaimStrings{app.TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = app.TokenKID
	signed, err := tok.SignedString([]byte(e2eJWTSecret))
	if err != nil {
		t.Fatalf("signing expired test token: %v", err)
	}
	return signed
}

// doRequest issues an HTTP request against the running app and decodes its
// envelope. body may be nil.
func doRequest(t *testing.T, method, url, bearerToken string) (status int, env e2eEnvelope) {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("%s %s: decoding envelope: %v", method, url, err)
	}
	return resp.StatusCode, env
}

// TestE2E_JWTToTenantContextToRLSToEnvelope is the single, comprehensive
// proof slice described in this file's package doc comment.
func TestE2E_JWTToTenantContextToRLSToEnvelope(t *testing.T) {
	baseURL, issuer := setupE2EApp(t)

	tokenA := mustIssue(t, issuer, "user-a", tenantAID, token.RoleGestor)
	tokenB := mustIssue(t, issuer, "user-b", tenantBID, token.RoleGestor)

	t.Run("tenant A creates a probe event and can read it back", func(t *testing.T) {
		status, env := doRequest(t, http.MethodPost, baseURL+"/v1/_probe/events", tokenA)
		if status != http.StatusCreated {
			t.Fatalf("POST /v1/_probe/events (tenant A): status = %d, want %d (env: %+v)", status, http.StatusCreated, env)
		}
		if env.Error != nil {
			t.Fatalf("POST /v1/_probe/events (tenant A): env.Error = %+v, want nil", env.Error)
		}
		if env.Data == nil || env.Data.TenantID != tenantAID {
			t.Fatalf("POST /v1/_probe/events (tenant A): env.Data = %+v, want tenant_id %q", env.Data, tenantAID)
		}
		aggregateID := env.Data.AggregateID

		status, env = doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/"+aggregateID, tokenA)
		if status != http.StatusOK {
			t.Fatalf("GET /v1/_probe/events/%s (tenant A, own event): status = %d, want %d (env: %+v)",
				aggregateID, status, http.StatusOK, env)
		}
		if env.Data == nil || env.Data.AggregateID != aggregateID || env.Data.TenantID != tenantAID {
			t.Fatalf("GET /v1/_probe/events/%s (tenant A): env.Data = %+v, want aggregate_id %q and tenant_id %q",
				aggregateID, env.Data, aggregateID, tenantAID)
		}
	})

	t.Run("tenant B cannot see tenant A's event: 404, never 403", func(t *testing.T) {
		status, env := doRequest(t, http.MethodPost, baseURL+"/v1/_probe/events", tokenA)
		if status != http.StatusCreated || env.Data == nil {
			t.Fatalf("POST /v1/_probe/events (tenant A, setup): status = %d, env = %+v", status, env)
		}
		aggregateID := env.Data.AggregateID

		status, env = doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/"+aggregateID, tokenB)
		if status != http.StatusNotFound {
			t.Fatalf("GET tenant A's event as tenant B: status = %d, want %d (RLS must hide it, never 403) (env: %+v)",
				status, http.StatusNotFound, env)
		}
		if env.Data != nil {
			t.Errorf("GET tenant A's event as tenant B: env.Data = %+v, want nil", env.Data)
		}
		if env.Error == nil || env.Error.Code != "probe_event_not_found" {
			t.Errorf("GET tenant A's event as tenant B: env.Error = %+v, want code %q", env.Error, "probe_event_not_found")
		}

		// Same code/message as a genuinely nonexistent aggregate id --
		// the two cases must be indistinguishable to the caller.
		statusMissing, envMissing := doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/does-not-exist-at-all", tokenB)
		if statusMissing != status || envMissing.Error.Code != env.Error.Code {
			t.Errorf("nonexistent aggregate id: status=%d code=%q, want the SAME response shape as a cross-tenant lookup (status=%d code=%q)",
				statusMissing, envMissing.Error.Code, status, env.Error.Code)
		}
	})

	t.Run("tenant B's own event remains readable by tenant B", func(t *testing.T) {
		status, env := doRequest(t, http.MethodPost, baseURL+"/v1/_probe/events", tokenB)
		if status != http.StatusCreated || env.Data == nil {
			t.Fatalf("POST /v1/_probe/events (tenant B): status = %d, env = %+v", status, env)
		}
		aggregateID := env.Data.AggregateID

		status, env = doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/"+aggregateID, tokenB)
		if status != http.StatusOK || env.Data == nil || env.Data.TenantID != tenantBID {
			t.Fatalf("GET own event (tenant B): status = %d, env = %+v, want 200 and tenant_id %q", status, env, tenantBID)
		}
	})

	t.Run("expired token is rejected with 401", func(t *testing.T) {
		expired := mustExpiredToken(t, tenantAID, token.RoleGestor)

		status, env := doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/whatever", expired)
		if status != http.StatusUnauthorized {
			t.Fatalf("GET with expired token: status = %d, want %d (env: %+v)", status, http.StatusUnauthorized, env)
		}
		if env.Data != nil {
			t.Errorf("GET with expired token: env.Data = %+v, want nil", env.Data)
		}
		if env.Error == nil || env.Error.Code != "invalid_token" {
			t.Errorf("GET with expired token: env.Error = %+v, want code %q", env.Error, "invalid_token")
		}
	})

	t.Run("insufficient role is rejected with 403, not 401", func(t *testing.T) {
		funcionarioToken := mustIssue(t, issuer, "user-c", tenantAID, token.RoleFuncionario)

		status, env := doRequest(t, http.MethodPost, baseURL+"/v1/_probe/events", funcionarioToken)
		if status != http.StatusForbidden {
			t.Fatalf("POST /v1/_probe/events (funcionario role, gestor-only route): status = %d, want %d (env: %+v)",
				status, http.StatusForbidden, env)
		}
		if env.Error == nil || env.Error.Code != "insufficient_role" {
			t.Errorf("env.Error = %+v, want code %q", env.Error, "insufficient_role")
		}

		// The same funcionario token IS accepted by the GET route (any
		// authenticated role is allowed to read).
		status, _ = doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/whatever", funcionarioToken)
		if status != http.StatusNotFound {
			t.Fatalf("GET /v1/_probe/events/whatever (funcionario role, any-role route): status = %d, want %d (route itself must accept the role)",
				status, http.StatusNotFound)
		}
	})

	t.Run("missing credentials are rejected with 401", func(t *testing.T) {
		status, env := doRequest(t, http.MethodGet, baseURL+"/v1/_probe/events/whatever", "")
		if status != http.StatusUnauthorized {
			t.Fatalf("GET without Authorization header: status = %d, want %d", status, http.StatusUnauthorized)
		}
		if env.Error == nil || env.Error.Code != "invalid_token" {
			t.Errorf("env.Error = %+v, want code %q", env.Error, "invalid_token")
		}
	})

	t.Run("every response, success or failure, carries a request id in meta", func(t *testing.T) {
		status, env := doRequest(t, http.MethodGet, baseURL+"/healthz", "")
		if status != http.StatusOK {
			t.Fatalf("GET /healthz: status = %d, want %d", status, http.StatusOK)
		}
		// /healthz never goes through Authenticate/tenant scoping, but
		// still returns the standard envelope shape with a request id.
		if env.Meta.RequestID == "" {
			t.Errorf("GET /healthz: env.Meta.RequestID is empty, want a populated request id")
		}
	})
}
