package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/app"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// testJWTSecret is a well-formed (>= 32 bytes), test-only signing secret --
// this package's tests never touch a real deployment's secret.
const testJWTSecret = "test-only-secret-at-least-32-bytes-long!!"

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		AppEnv:    "development",
		Port:      "0",
		JWTSecret: testJWTSecret,
	}
}

func newTestApp(t *testing.T) http.Handler {
	t.Helper()
	cfg := testConfig(t)
	verifier, err := app.BuildVerifier(cfg)
	if err != nil {
		t.Fatalf("app.BuildVerifier: %v", err)
	}
	issuer, err := app.BuildIssuer(cfg)
	if err != nil {
		t.Fatalf("app.BuildIssuer: %v", err)
	}
	// pool is intentionally nil: every test in this file either hits
	// /healthz (which never touches the database) or a request Authenticate/
	// RequireRole rejects before it ever reaches a database-backed handler.
	return app.NewApp(cfg, nil, nil, verifier, issuer, nil)
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) httpx.Envelope {
	t.Helper()
	var env httpx.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding response envelope: %v (body: %s)", err, rec.Body.String())
	}
	return env
}

func TestNewApp_Healthz_NoDatabaseRequired(t *testing.T) {
	handler := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if env.Error != nil {
		t.Fatalf("GET /healthz: env.Error = %+v, want nil", env.Error)
	}
}

func TestNewApp_ProbeEndToEnd_ViaHTTPTestServer(t *testing.T) {
	// Proves NewApp's result is a genuine, servable http.Handler -- not
	// just something that satisfies the interface in isolation -- by
	// running it behind a real httptest.Server and issuing a real HTTP
	// request over a real (loopback) connection.
	srv := httptest.NewServer(newTestApp(t))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestNewApp_Probe_MissingAuthorizationHeader_Returns401(t *testing.T) {
	handler := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/_probe/events/does-not-matter", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /v1/_probe/events/... (no Authorization header): status = %d, want %d (body: %s)",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if env.Data != nil {
		t.Errorf("env.Data = %v, want nil", env.Data)
	}
	if env.Error == nil {
		t.Fatal("env.Error = nil, want a populated error body")
	}
	if env.Error.Code != "invalid_token" {
		t.Errorf("env.Error.Code = %q, want %q", env.Error.Code, "invalid_token")
	}
}

func TestNewApp_Probe_MalformedBearerScheme_Returns401(t *testing.T) {
	handler := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/_probe/events/does-not-matter", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET with Basic scheme: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// signToken builds a raw JWT signed with testJWTSecret and app.TokenKID,
// letting each test override the claims it cares about (in particular,
// ExpiresAt) -- something app.BuildIssuer's fixed AccessTokenTTL cannot
// produce (an already-expired token) on its own.
func signToken(t *testing.T, claims token.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = app.TokenKID
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("signing test token: %v", err)
	}
	return signed
}

func validClaims(role token.Role) token.Claims {
	now := time.Now().UTC()
	return token.Claims{
		TenantID:  "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Role:      role,
		SessionID: "session-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			Issuer:    app.TokenIssuer,
			Audience:  jwt.ClaimStrings{app.TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(token.AccessTokenTTL)),
		},
	}
}

func TestNewApp_Probe_ExpiredToken_Returns401(t *testing.T) {
	handler := newTestApp(t)

	claims := validClaims(token.RoleGestor)
	now := time.Now().UTC()
	claims.IssuedAt = jwt.NewNumericDate(now.Add(-2 * time.Hour))
	claims.NotBefore = jwt.NewNumericDate(now.Add(-2 * time.Hour))
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))
	expired := signToken(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/v1/_probe/events/does-not-matter", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET with expired token: status = %d, want %d (body: %s)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if env.Error == nil || env.Error.Code != "invalid_token" {
		t.Fatalf("GET with expired token: env.Error = %+v, want code %q", env.Error, "invalid_token")
	}
}

func TestNewApp_Probe_InsufficientRole_Returns403(t *testing.T) {
	handler := newTestApp(t)

	// POST /v1/_probe/events is restricted to RoleGestor; a well-formed,
	// non-expired token for RoleFuncionario must be rejected with 403, not
	// 401 or 500.
	funcionarioToken := signToken(t, validClaims(token.RoleFuncionario))

	req := httptest.NewRequest(http.MethodPost, "/v1/_probe/events", nil)
	req.Header.Set("Authorization", "Bearer "+funcionarioToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /v1/_probe/events (funcionario role): status = %d, want %d (body: %s)",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if env.Error == nil || env.Error.Code != "insufficient_role" {
		t.Fatalf("env.Error = %+v, want code %q", env.Error, "insufficient_role")
	}
}

func TestNewApp_Probe_UnknownRoute_Returns404Envelope(t *testing.T) {
	handler := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/does-not-exist: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestBuildVerifierAndBuildIssuer_AgreeOnKIDIssuerAudience(t *testing.T) {
	cfg := testConfig(t)

	issuer, err := app.BuildIssuer(cfg)
	if err != nil {
		t.Fatalf("app.BuildIssuer: %v", err)
	}
	verifier, err := app.BuildVerifier(cfg)
	if err != nil {
		t.Fatalf("app.BuildVerifier: %v", err)
	}

	signed, err := issuer.Issue("user-1", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", token.RoleGestor, "session-1")
	if err != nil {
		t.Fatalf("issuer.Issue: %v", err)
	}

	claims, err := verifier.Verify(signed)
	if err != nil {
		t.Fatalf("verifier.Verify(a token minted by BuildIssuer): %v", err)
	}
	if claims.TenantID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("claims.TenantID = %q, want the tenant id passed to Issue", claims.TenantID)
	}
}
