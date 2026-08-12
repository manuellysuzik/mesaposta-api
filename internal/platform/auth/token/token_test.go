package token_test

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const (
	testKID      = "test-kid-1"
	testIssuer   = "mesaposta-api-test"
	testAudience = "mesaposta-api-test-clients"
)

func testSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef") // 33 bytes, >= 32 floor
}

func newTestIssuer(t *testing.T) *token.Issuer {
	t.Helper()
	iss, err := token.NewIssuer(testKID, testSecret(), testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewIssuer() returned error: %v", err)
	}
	return iss
}

func newTestVerifier(t *testing.T, keys map[string][]byte) *token.Verifier {
	t.Helper()
	v, err := token.NewVerifier(keys, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewVerifier() returned error: %v", err)
	}
	return v
}

// signRaw builds and signs a token directly (bypassing Issuer.Issue) so
// tests can construct edge-case claims (expired, missing tenant_id, wrong
// iss/aud, clock-skewed times, ...) that the well-behaved Issuer would
// never produce.
func signRaw(t *testing.T, method jwt.SigningMethod, key any, kid string, claims token.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString() returned error: %v", err)
	}
	return signed
}

func validClaims(now time.Time) token.Claims {
	return token.Claims{
		TenantID:  "11111111-1111-1111-1111-111111111111",
		Role:      token.RoleGestor,
		SessionID: "session-abc",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(token.AccessTokenTTL)),
		},
	}
}

func TestIssueAndVerify_RoundTripValidToken(t *testing.T) {
	issuer := newTestIssuer(t)
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})

	signed, err := issuer.Issue("user-42", "11111111-1111-1111-1111-111111111111", token.RoleGestor, "session-abc")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	claims, err := verifier.Verify(signed)
	if err != nil {
		t.Fatalf("Verify() returned error: %v", err)
	}

	if claims.Subject != "user-42" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-42")
	}
	if claims.TenantID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("TenantID = %q, want %q", claims.TenantID, "11111111-1111-1111-1111-111111111111")
	}
	if claims.Role != token.RoleGestor {
		t.Errorf("Role = %q, want %q", claims.Role, token.RoleGestor)
	}
	if claims.SessionID != "session-abc" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "session-abc")
	}
}

func TestVerify_ExpiredTokenIsUnauthenticated(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})

	now := time.Now()
	claims := validClaims(now)
	claims.IssuedAt = jwt.NewNumericDate(now.Add(-2 * time.Hour))
	claims.NotBefore = jwt.NewNumericDate(now.Add(-2 * time.Hour))
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(-1 * time.Hour))

	signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

	_, err := verifier.Verify(signed)
	if err == nil {
		t.Fatal("Verify() with an expired token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestVerify_TamperedSignatureIsUnauthenticated(t *testing.T) {
	issuer := newTestIssuer(t)
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})

	signed, err := issuer.Issue("user-42", "11111111-1111-1111-1111-111111111111", token.RoleGestor, "session-abc")
	if err != nil {
		t.Fatalf("Issue() returned error: %v", err)
	}

	tampered := tamperLastChar(signed)

	_, err = verifier.Verify(tampered)
	if err == nil {
		t.Fatal("Verify() with a tampered signature: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func tamperLastChar(s string) string {
	if s[len(s)-1] == 'A' {
		return s[:len(s)-1] + "B"
	}
	return s[:len(s)-1] + "A"
}

func TestVerify_RejectsAlgNone(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})

	now := time.Now()
	claims := validClaims(now)

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString() with alg:none returned error: %v", err)
	}

	// Belt and suspenders: confirm the crafted token really does carry
	// alg:none before asserting our verifier rejects it, so this test
	// can't pass vacuously if the crafting step silently changed shape.
	if !strings.HasPrefix(signed, "eyJhbGciOiJub25lIi") {
		t.Fatalf("crafted token does not look like an alg:none token: %s", signed)
	}

	_, err = verifier.Verify(signed)
	if err == nil {
		t.Fatal("Verify() with alg:none: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestVerify_RejectsRS256TokenPresentedToHS256Validator(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() returned error: %v", err)
	}

	now := time.Now()
	claims := validClaims(now)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(rsaKey)
	if err != nil {
		t.Fatalf("SignedString() with RS256 returned error: %v", err)
	}

	_, err = verifier.Verify(signed)
	if err == nil {
		t.Fatal("Verify() with an RS256-signed token: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
	}
}

func TestVerify_UnknownOrMissingKidIsUnauthenticated(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})
	now := time.Now()
	claims := validClaims(now)

	tests := []struct {
		name string
		kid  string
	}{
		{"unknown kid", "some-other-kid"},
		{"missing kid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), tt.kid, claims)

			_, err := verifier.Verify(signed)
			if err == nil {
				t.Fatal("Verify(): expected error, got nil")
			}
			if got := errs.KindOf(err); got != errs.Unauthenticated {
				t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
			}
		})
	}
}

func TestVerify_MissingOrMalformedTenantIDIsUnauthenticatedNotInvalid(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})
	now := time.Now()

	claims := validClaims(now)
	claims.TenantID = ""
	signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

	_, err := verifier.Verify(signed)
	if err == nil {
		t.Fatal("Verify() with empty tenant_id: expected error, got nil")
	}
	if got := errs.KindOf(err); got != errs.Unauthenticated {
		t.Errorf("errs.KindOf(err) = %v, want %v (must not leak Invalid vs Unauthenticated distinction)", got, errs.Unauthenticated)
	}
	if errs.IsKind(err, errs.Invalid) {
		t.Error("errs.IsKind(err, errs.Invalid) = true; a bad tenant_id must classify as Unauthenticated, never Invalid")
	}
}

func TestVerify_WrongIssuerOrAudienceIsUnauthenticated(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})
	now := time.Now()

	tests := []struct {
		name   string
		mutate func(c *token.Claims)
	}{
		{"wrong issuer", func(c *token.Claims) { c.Issuer = "someone-else" }},
		{"wrong audience", func(c *token.Claims) { c.Audience = jwt.ClaimStrings{"someone-elses-clients"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := validClaims(now)
			tt.mutate(&claims)
			signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

			_, err := verifier.Verify(signed)
			if err == nil {
				t.Fatal("Verify(): expected error, got nil")
			}
			if got := errs.KindOf(err); got != errs.Unauthenticated {
				t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
			}
		})
	}
}

func TestVerify_ClockSkewToleratedUpTo30SecondsButNotBeyond(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})
	now := time.Now()

	t.Run("expired 20s ago is within the 30s leeway", func(t *testing.T) {
		claims := validClaims(now)
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(-20 * time.Second))
		signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

		if _, err := verifier.Verify(signed); err != nil {
			t.Fatalf("Verify() within 30s leeway returned error: %v", err)
		}
	})

	t.Run("expired 40s ago exceeds the 30s leeway", func(t *testing.T) {
		claims := validClaims(now)
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(-40 * time.Second))
		signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

		_, err := verifier.Verify(signed)
		if err == nil {
			t.Fatal("Verify() beyond 30s leeway: expected error, got nil")
		}
		if got := errs.KindOf(err); got != errs.Unauthenticated {
			t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
		}
	})

	t.Run("not-before 20s in the future is within the 30s leeway", func(t *testing.T) {
		claims := validClaims(now)
		claims.NotBefore = jwt.NewNumericDate(now.Add(20 * time.Second))
		signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

		if _, err := verifier.Verify(signed); err != nil {
			t.Fatalf("Verify() within 30s leeway returned error: %v", err)
		}
	})

	t.Run("not-before 40s in the future exceeds the 30s leeway", func(t *testing.T) {
		claims := validClaims(now)
		claims.NotBefore = jwt.NewNumericDate(now.Add(40 * time.Second))
		signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

		_, err := verifier.Verify(signed)
		if err == nil {
			t.Fatal("Verify() beyond 30s leeway: expected error, got nil")
		}
		if got := errs.KindOf(err); got != errs.Unauthenticated {
			t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
		}
	})
}

func TestVerify_UnknownOrEmptyRoleIsUnauthenticated(t *testing.T) {
	verifier := newTestVerifier(t, map[string][]byte{testKID: testSecret()})
	now := time.Now()

	tests := []struct {
		name string
		role token.Role
	}{
		{"unknown role", token.Role("super-admin")},
		{"empty role", token.Role("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := validClaims(now)
			claims.Role = tt.role
			signed := signRaw(t, jwt.SigningMethodHS256, testSecret(), testKID, claims)

			_, err := verifier.Verify(signed)
			if err == nil {
				t.Fatal("Verify(): expected error, got nil")
			}
			if got := errs.KindOf(err); got != errs.Unauthenticated {
				t.Errorf("errs.KindOf(err) = %v, want %v", got, errs.Unauthenticated)
			}
		})
	}
}

func TestRole_Valid(t *testing.T) {
	tests := []struct {
		role token.Role
		want bool
	}{
		{token.RoleGestor, true},
		{token.RoleFuncionario, true},
		{token.Role(""), false},
		{token.Role("unknown"), false},
	}

	for _, tt := range tests {
		if got := tt.role.Valid(); got != tt.want {
			t.Errorf("Role(%q).Valid() = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestNewIssuer_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		kid      string
		secret   []byte
		issuer   string
		audience string
	}{
		{"empty kid", "", testSecret(), testIssuer, testAudience},
		{"short secret", testKID, []byte("too-short"), testIssuer, testAudience},
		{"empty issuer", testKID, testSecret(), "", testAudience},
		{"empty audience", testKID, testSecret(), testIssuer, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := token.NewIssuer(tt.kid, tt.secret, tt.issuer, tt.audience); err == nil {
				t.Fatal("NewIssuer(): expected error, got nil")
			}
		})
	}
}

func TestNewVerifier_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		keys     map[string][]byte
		issuer   string
		audience string
	}{
		{"no keys", map[string][]byte{}, testIssuer, testAudience},
		{"short secret", map[string][]byte{testKID: []byte("too-short")}, testIssuer, testAudience},
		{"empty issuer", map[string][]byte{testKID: testSecret()}, "", testAudience},
		{"empty audience", map[string][]byte{testKID: testSecret()}, testIssuer, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := token.NewVerifier(tt.keys, tt.issuer, tt.audience); err == nil {
				t.Fatal("NewVerifier(): expected error, got nil")
			}
		})
	}
}

func TestIssue_RejectsInvalidInput(t *testing.T) {
	issuer := newTestIssuer(t)

	tests := []struct {
		name      string
		subject   string
		tenantID  string
		role      token.Role
		sessionID string
	}{
		{"empty subject", "", "tenant-1", token.RoleGestor, "session-1"},
		{"empty tenant id", "user-1", "", token.RoleGestor, "session-1"},
		{"invalid role", "user-1", "tenant-1", token.Role("bogus"), "session-1"},
		{"empty session id", "user-1", "tenant-1", token.RoleGestor, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issuer.Issue(tt.subject, tt.tenantID, tt.role, tt.sessionID); err == nil {
				t.Fatal("Issue(): expected error, got nil")
			}
		})
	}
}
