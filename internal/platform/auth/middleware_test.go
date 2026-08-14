package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const (
	testIssuer   = "mesaposta-api"
	testAudience = "mesaposta-clients"

	tenantAUUID = "11111111-1111-1111-1111-111111111111"
	tenantBUUID = "22222222-2222-2222-2222-222222222222"
)

func newTestTokenPair(t *testing.T) (*token.Issuer, *token.Verifier) {
	t.Helper()
	secret := bytes.Repeat([]byte("k"), 32)

	issuer, err := token.NewIssuer("kid-1", secret, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	verifier, err := token.NewVerifier(map[string][]byte{"kid-1": secret}, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return issuer, verifier
}

// probeResult records what a downstream handler actually observed in its
// request context, so tests can assert on what really reached the handler
// rather than on the middleware's internal behavior.
type probeResult struct {
	called    bool
	tenantID  string
	tenantErr error
	role      token.Role
	hasClaims bool
}

func probeHandler(result *probeResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result.called = true

		tid, err := tenant.From(r.Context())
		result.tenantErr = err
		if err == nil {
			result.tenantID = tid.String()
		}

		if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
			result.hasClaims = true
			result.role = claims.Role
		}

		w.WriteHeader(http.StatusOK)
	}
}

func TestAuthenticate_MissingAuthorizationHeader_Returns401AndNeverCallsHandler(t *testing.T) {
	_, verifier := newTestTokenPair(t)
	result := &probeResult{}
	handler := auth.Authenticate(verifier)(probeHandler(result))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if result.called {
		t.Fatalf("downstream handler must never execute without a valid Authorization header")
	}
}

func TestAuthenticate_WrongSchemeOrEmptyToken_Returns401AndNeverCallsHandler(t *testing.T) {
	_, verifier := newTestTokenPair(t)

	cases := []struct {
		name   string
		header string
	}{
		{"basic scheme", "Basic dXNlcjpwYXNz"},
		{"lowercase bearer", "bearer sometoken"},
		{"Bearer with no token", "Bearer"},
		{"Bearer with only trailing space", "Bearer "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &probeResult{}
			handler := auth.Authenticate(verifier)(probeHandler(result))

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tc.header)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 for header %q", rec.Code, tc.header)
			}
			if result.called {
				t.Fatalf("downstream handler must never execute for header %q", tc.header)
			}
		})
	}
}

func TestAuthenticate_ValidToken_ExposesExactTenantIDToHandler(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	signed, err := issuer.Issue("user-1", tenantAUUID, token.RoleGestor, "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	result := &probeResult{}
	handler := auth.Authenticate(verifier)(probeHandler(result))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !result.called {
		t.Fatalf("downstream handler should have executed")
	}
	if result.tenantErr != nil {
		t.Fatalf("tenant.From returned error: %v", result.tenantErr)
	}
	if result.tenantID != tenantAUUID {
		t.Fatalf("tenant.From = %q, want %q", result.tenantID, tenantAUUID)
	}
	if !result.hasClaims || result.role != token.RoleGestor {
		t.Fatalf("expected claims with role %q, got hasClaims=%v role=%q", token.RoleGestor, result.hasClaims, result.role)
	}
}

func TestAuthenticate_MalformedTenantIDClaim_Returns401NotServerError(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	// The token itself is well-formed and validly signed; only the
	// tenant_id claim is not a well-formed UUID. token.Verify does not
	// call tenant.Parse (see token/claims.go's doc comment), so this token
	// verifies successfully -- the failure must surface inside
	// Authenticate, when it calls tenant.Parse, and must be reported as an
	// authentication failure (401), never a 500.
	signed, err := issuer.Issue("user-1", "not-a-valid-uuid", token.RoleGestor, "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	result := &probeResult{}
	handler := auth.Authenticate(verifier)(probeHandler(result))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (never 500) for a malformed tenant_id claim, body=%s", rec.Code, rec.Body.String())
	}
	if result.called {
		t.Fatalf("downstream handler must never execute when tenant_id fails to parse")
	}
}

func TestRequireRole_InsufficientRole_Returns403(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	signed, err := issuer.Issue("user-1", tenantAUUID, token.RoleFuncionario, "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	result := &probeResult{}
	handler := auth.Authenticate(verifier)(
		auth.RequireRole(token.RoleGestor)(probeHandler(result)),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if result.called {
		t.Fatalf("downstream handler must never execute when role is insufficient")
	}
}

func TestRequireRole_SufficientRole_CallsHandler(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	signed, err := issuer.Issue("user-1", tenantAUUID, token.RoleGestor, "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	result := &probeResult{}
	handler := auth.Authenticate(verifier)(
		auth.RequireRole(token.RoleGestor, token.RoleFuncionario)(probeHandler(result)),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !result.called {
		t.Fatalf("downstream handler should have executed")
	}
}

// TestAuthenticate_TenantIsolation_TokenFromOneTenantNeverLeaksAnothers
// proves a token issued for tenant A never produces a context carrying
// tenant B's id, and vice-versa -- guarding against any accidental shared
// mutable state (e.g. a package-level variable) in the middleware.
func TestAuthenticate_TenantIsolation_TokenFromOneTenantNeverLeaksAnothers(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	tokenA, err := issuer.Issue("user-a", tenantAUUID, token.RoleGestor, "session-a")
	if err != nil {
		t.Fatalf("Issue tokenA: %v", err)
	}
	tokenB, err := issuer.Issue("user-b", tenantBUUID, token.RoleGestor, "session-b")
	if err != nil {
		t.Fatalf("Issue tokenB: %v", err)
	}

	resultA := &probeResult{}
	handlerA := auth.Authenticate(verifier)(probeHandler(resultA))
	reqA := httptest.NewRequest("GET", "/", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	recA := httptest.NewRecorder()
	handlerA.ServeHTTP(recA, reqA)

	resultB := &probeResult{}
	handlerB := auth.Authenticate(verifier)(probeHandler(resultB))
	reqB := httptest.NewRequest("GET", "/", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	recB := httptest.NewRecorder()
	handlerB.ServeHTTP(recB, reqB)

	if resultA.tenantID != tenantAUUID {
		t.Fatalf("request A saw tenant %q, want %q", resultA.tenantID, tenantAUUID)
	}
	if resultB.tenantID != tenantBUUID {
		t.Fatalf("request B saw tenant %q, want %q", resultB.tenantID, tenantBUUID)
	}
	if resultA.tenantID == resultB.tenantID {
		t.Fatalf("tenant isolation violated: both requests saw the same tenant id %q", resultA.tenantID)
	}
}

// TestAuthenticate_NeverLogsRawToken proves that running Authenticate as
// part of a full request pipeline (mirroring real wiring: httpx.NewRouter's
// structured access logger + auth.Authenticate on a protected route) never
// writes the raw JWT string to the log output, for both a successful and a
// rejected authentication attempt.
func TestAuthenticate_NeverLogsRawToken(t *testing.T) {
	issuer, verifier := newTestTokenPair(t)

	validToken, err := issuer.Issue("user-1", tenantAUUID, token.RoleGestor, "session-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	alwaysReady := func(ctx context.Context) error { return nil }

	router := httpx.NewRouter(logger, alwaysReady, time.Second, nil)
	router.With(auth.Authenticate(verifier)).Get("/protected", func(w http.ResponseWriter, r *http.Request) {
		httpx.RespondData(w, r, http.StatusOK, map[string]string{"ok": "yes"}, nil)
	})

	// A successful authentication.
	reqOK := httptest.NewRequest("GET", "/protected", nil)
	reqOK.Header.Set("Authorization", "Bearer "+validToken)
	recOK := httptest.NewRecorder()
	router.ServeHTTP(recOK, reqOK)
	if recOK.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d: %s", recOK.Code, recOK.Body.String())
	}

	// A rejected authentication (garbage bearer value, but still logged as
	// part of the access log line for the request).
	reqBad := httptest.NewRequest("GET", "/protected", nil)
	reqBad.Header.Set("Authorization", "Bearer this.is.not-a-valid-jwt-garbage-token")
	recBad := httptest.NewRecorder()
	router.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for garbage token, got %d: %s", recBad.Code, recBad.Body.String())
	}

	logOutput := logBuf.String()
	for _, rawToken := range []string{validToken, "this.is.not-a-valid-jwt-garbage-token"} {
		if strings.Contains(logOutput, rawToken) {
			t.Fatalf("raw token %q leaked into log output: %s", rawToken, logOutput)
		}
	}

	var decodedForSanity map[string]json.RawMessage
	if err := json.Unmarshal(bytes.Split(bytes.TrimSpace(logBuf.Bytes()), []byte("\n"))[0], &decodedForSanity); err != nil {
		t.Fatalf("log output is not valid JSON lines: %v (%s)", err, logOutput)
	}
}
