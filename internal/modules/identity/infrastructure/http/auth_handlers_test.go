package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	identityhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/identity/infrastructure/http"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
)

const (
	testTokenKID      = "primary"
	testTokenIssuer   = "mesaposta-api"
	testTokenAudience = "mesaposta-api"
	testJWTSecret     = "this-is-a-perfectly-fine-32-byte-plus-secret"
)

// fakeCredentialsLookup is a hand-written, in-memory implementation of
// application.UserCredentialsLookup for this package's httptest-based
// handler tests -- mirrors the fake-implements-the-port pattern
// ../../../catalog/infrastructure/http's own fake_uow_test.go establishes.
type fakeCredentialsLookup struct {
	byEmail map[string]db.UserCredentials
}

func newFakeCredentialsLookup() *fakeCredentialsLookup {
	return &fakeCredentialsLookup{byEmail: map[string]db.UserCredentials{}}
}

func (f *fakeCredentialsLookup) LookupUserCredentialsByEmail(_ context.Context, email string) (db.UserCredentials, error) {
	creds, ok := f.byEmail[email]
	if !ok {
		return db.UserCredentials{}, errNotFound
	}
	return creds, nil
}

var errNotFound = &notFoundErr{}

type notFoundErr struct{}

func (e *notFoundErr) Error() string { return "user not found" }

func mustHashPassword(t *testing.T, plain string) string {
	t.Helper()
	hash, err := password.Hash(plain)
	if err != nil {
		t.Fatalf("password.Hash(%q): %v", plain, err)
	}
	return hash
}

func newTestIssuer(t *testing.T) *token.Issuer {
	t.Helper()
	issuer, err := token.NewIssuer(testTokenKID, []byte(testJWTSecret), testTokenIssuer, testTokenAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	return issuer
}

func newTestVerifier(t *testing.T) *token.Verifier {
	t.Helper()
	verifier, err := token.NewVerifier(map[string][]byte{testTokenKID: []byte(testJWTSecret)}, testTokenIssuer, testTokenAudience)
	if err != nil {
		t.Fatalf("token.NewVerifier: %v", err)
	}
	return verifier
}

func newTestRouter(lookup *fakeCredentialsLookup, issuer *token.Issuer) chi.Router {
	r := chi.NewRouter()
	identityhttp.Mount(r, lookup, issuer)
	return r
}

func doLoginRequest(r chi.Router, email, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeLoginEnvelope(t *testing.T, body *bytes.Buffer) map[string]json.RawMessage {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response envelope: %v (body=%s)", err, body.String())
	}
	return envelope
}

func TestLoginHandler_ValidCredentials_Returns200WithVerifiableToken(t *testing.T) {
	lookup := newFakeCredentialsLookup()
	lookup.byEmail["manager@example.com"] = db.UserCredentials{
		ID:           "aaaaaaaa-bbbb-cccc-dddd-000000000001",
		TenantID:     "aaaaaaaa-bbbb-cccc-dddd-000000000002",
		Email:        "manager@example.com",
		PasswordHash: mustHashPassword(t, "correct-horse-battery-staple"),
		Role:         "manager",
		IsActive:     true,
	}
	issuer := newTestIssuer(t)
	router := newTestRouter(lookup, issuer)

	w := doLoginRequest(router, "manager@example.com", "correct-horse-battery-staple")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	envelope := decodeLoginEnvelope(t, w.Body)
	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Role        string `json:"role"`
		UserID      string `json:"user_id"`
		TenantID    string `json:"tenant_id"`
	}
	if err := json.Unmarshal(envelope["data"], &resp); err != nil {
		t.Fatalf("decoding data: %v", err)
	}

	if resp.AccessToken == "" {
		t.Fatal("access_token must not be empty")
	}
	if resp.Role != "manager" {
		t.Errorf("role = %q, want %q", resp.Role, "manager")
	}
	if resp.UserID != "aaaaaaaa-bbbb-cccc-dddd-000000000001" {
		t.Errorf("user_id = %q, want the fake user's id", resp.UserID)
	}
	if resp.ExpiresIn != int64(token.AccessTokenTTL.Seconds()) {
		t.Errorf("expires_in = %d, want %d", resp.ExpiresIn, int64(token.AccessTokenTTL.Seconds()))
	}

	verifier := newTestVerifier(t)
	claims, err := verifier.Verify(resp.AccessToken)
	if err != nil {
		t.Fatalf("verifier.Verify(access_token): %v", err)
	}
	if claims.TenantID != resp.TenantID {
		t.Errorf("verified claims tenant_id = %q, want %q", claims.TenantID, resp.TenantID)
	}
	if string(claims.Role) != "manager" {
		t.Errorf("verified claims role = %q, want %q", claims.Role, "manager")
	}
}

func TestLoginHandler_UnknownEmail_Returns401Uniform(t *testing.T) {
	lookup := newFakeCredentialsLookup()
	router := newTestRouter(lookup, newTestIssuer(t))

	w := doLoginRequest(router, "nobody@example.com", "whatever")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	assertUniformInvalidCredentials(t, w.Body)
}

func TestLoginHandler_WrongPassword_Returns401Uniform(t *testing.T) {
	lookup := newFakeCredentialsLookup()
	lookup.byEmail["employee@example.com"] = db.UserCredentials{
		ID:           "aaaaaaaa-bbbb-cccc-dddd-000000000003",
		TenantID:     "aaaaaaaa-bbbb-cccc-dddd-000000000002",
		Email:        "employee@example.com",
		PasswordHash: mustHashPassword(t, "the-real-password"),
		Role:         "employee",
		IsActive:     true,
	}
	router := newTestRouter(lookup, newTestIssuer(t))

	w := doLoginRequest(router, "employee@example.com", "wrong-password")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	assertUniformInvalidCredentials(t, w.Body)
}

func TestLoginHandler_InactiveUser_Returns401Uniform(t *testing.T) {
	lookup := newFakeCredentialsLookup()
	lookup.byEmail["deactivated@example.com"] = db.UserCredentials{
		ID:           "aaaaaaaa-bbbb-cccc-dddd-000000000004",
		TenantID:     "aaaaaaaa-bbbb-cccc-dddd-000000000002",
		Email:        "deactivated@example.com",
		PasswordHash: mustHashPassword(t, "correct-password"),
		Role:         "manager",
		IsActive:     false,
	}
	router := newTestRouter(lookup, newTestIssuer(t))

	w := doLoginRequest(router, "deactivated@example.com", "correct-password")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	assertUniformInvalidCredentials(t, w.Body)
}

// assertUniformInvalidCredentials checks the error envelope matches the
// single, uniform code every AuthenticateUser failure mode must produce --
// see application.AuthenticateUser's own doc comment on why no failure mode
// may be distinguishable from any other in the HTTP response.
func assertUniformInvalidCredentials(t *testing.T, body *bytes.Buffer) {
	t.Helper()
	envelope := decodeLoginEnvelope(t, body)
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(envelope["error"], &errResp); err != nil {
		t.Fatalf("decoding error: %v", err)
	}
	if errResp.Code != "invalid_credentials" {
		t.Errorf("error.code = %q, want %q", errResp.Code, "invalid_credentials")
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Error("error.message must not be empty")
	}
}
