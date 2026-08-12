// Package token implements mesaposta-api's short-lived access tokens: HS256
// JWTs, valid for AccessTokenTTL (15 minutes), carrying a Claims payload
// (subject, tenant, role, session id) and a "kid" header from day one so
// the signing secret can be rotated later without a breaking change.
//
// Verify is deliberately strict about the "alg" a token claims to use: it
// always fixes HS256 as the only accepted signing method, rather than
// trusting whatever the token's own header says, precisely to defend
// against algorithm-confusion attacks (a hand-crafted alg:none token, or a
// token legitimately signed with a different algorithm such as RS256
// presented to this HS256-only validator). See Verifier.Verify.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const (
	// AccessTokenTTL is the fixed lifetime of every access token this
	// package issues.
	AccessTokenTTL = 15 * time.Minute

	// clockSkewLeeway is the maximum clock skew tolerated between issuer
	// and verifier when checking exp/nbf: no more than 30 seconds.
	clockSkewLeeway = 30 * time.Second

	// minSecretLen is the minimum accepted HMAC secret length, in bytes
	// (256 bits), so the signing key is never the weak link.
	minSecretLen = 32
)

// signingMethod is the single algorithm this package ever signs with or
// accepts.
var signingMethod = jwt.SigningMethodHS256

// errUnexpectedSigningMethod and errUnknownKeyID are returned by
// Verifier's internal jwt.Keyfunc; Verify collapses both (and every other
// failure) into a single, uniform *errs.Error via unauthenticated.
var (
	errUnexpectedSigningMethod = errors.New("token: unexpected signing method")
	errUnknownKeyID            = errors.New("token: unknown or missing kid")
)

// Issuer mints access tokens signed with a single HMAC secret, identified
// by kid (key id) in the token header so a Verifier can be configured with
// multiple keys during secret rotation.
type Issuer struct {
	kid      string
	secret   []byte
	issuer   string
	audience string
}

// NewIssuer builds an Issuer. kid, issuer and audience must be non-empty;
// secret must be at least 32 bytes (256 bits) of key material.
func NewIssuer(kid string, secret []byte, issuer, audience string) (*Issuer, error) {
	if kid == "" {
		return nil, errs.New(errs.Invalid, "invalid_issuer_kid", "issuer kid must not be empty", nil)
	}
	if len(secret) < minSecretLen {
		return nil, errs.New(errs.Invalid, "invalid_issuer_secret",
			fmt.Sprintf("issuer secret must be at least %d bytes", minSecretLen), nil)
	}
	if issuer == "" {
		return nil, errs.New(errs.Invalid, "invalid_issuer_iss", "issuer name must not be empty", nil)
	}
	if audience == "" {
		return nil, errs.New(errs.Invalid, "invalid_issuer_aud", "issuer audience must not be empty", nil)
	}

	secretCopy := append([]byte(nil), secret...)
	return &Issuer{kid: kid, secret: secretCopy, issuer: issuer, audience: audience}, nil
}

// Issue mints a new, signed access token for the given subject/tenant/
// role/session, valid for AccessTokenTTL starting now.
func (i *Issuer) Issue(subject, tenantID string, role Role, sessionID string) (string, error) {
	if subject == "" {
		return "", errs.New(errs.Invalid, "invalid_claims_subject", "subject must not be empty", nil)
	}
	if tenantID == "" {
		return "", errs.New(errs.Invalid, "invalid_claims_tenant_id", "tenant id must not be empty", nil)
	}
	if !role.Valid() {
		return "", errs.New(errs.Invalid, "invalid_claims_role", "role must be a known, non-empty value", nil)
	}
	if sessionID == "" {
		return "", errs.New(errs.Invalid, "invalid_claims_session_id", "session id must not be empty", nil)
	}

	now := time.Now().UTC()
	claims := Claims{
		TenantID:  tenantID,
		Role:      role,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    i.issuer,
			Audience:  jwt.ClaimStrings{i.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}

	tok := jwt.NewWithClaims(signingMethod, claims)
	tok.Header["kid"] = i.kid

	signed, err := tok.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("token: signing: %w", err)
	}
	return signed, nil
}

// Verifier validates access tokens issued by an Issuer -- including, once
// the signing secret has been rotated, tokens issued by a *previous*
// Issuer, as long as Verifier is configured with that old kid's secret
// alongside the current one.
type Verifier struct {
	keys     map[string][]byte
	issuer   string
	audience string
}

// NewVerifier builds a Verifier accepting tokens signed with any of the
// given kid -> secret pairs. keys must be non-empty and every secret must
// be at least 32 bytes; issuer and audience must be non-empty.
func NewVerifier(keys map[string][]byte, issuer, audience string) (*Verifier, error) {
	if len(keys) == 0 {
		return nil, errs.New(errs.Invalid, "invalid_verifier_keys", "verifier requires at least one signing key", nil)
	}
	for kid, secret := range keys {
		if kid == "" {
			return nil, errs.New(errs.Invalid, "invalid_verifier_kid", "verifier key id must not be empty", nil)
		}
		if len(secret) < minSecretLen {
			return nil, errs.New(errs.Invalid, "invalid_verifier_secret",
				fmt.Sprintf("verifier secret for kid %q must be at least %d bytes", kid, minSecretLen), nil)
		}
	}
	if issuer == "" {
		return nil, errs.New(errs.Invalid, "invalid_verifier_iss", "verifier issuer must not be empty", nil)
	}
	if audience == "" {
		return nil, errs.New(errs.Invalid, "invalid_verifier_aud", "verifier audience must not be empty", nil)
	}

	keysCopy := make(map[string][]byte, len(keys))
	for kid, secret := range keys {
		keysCopy[kid] = append([]byte(nil), secret...)
	}

	return &Verifier{keys: keysCopy, issuer: issuer, audience: audience}, nil
}

// unauthenticated wraps cause as the single public-facing error every
// Verify failure is reported as, regardless of its specific root cause
// (expired signature, algorithm confusion, unknown kid, wrong issuer/
// audience, malformed/missing tenant_id, unknown role, ...). Collapsing
// every failure mode into one Kind/Code/message is deliberate: it denies a
// caller any oracle to distinguish "well-formed token, rejected for
// reason X" from "garbage input" -- in particular, a bad tenant_id must
// never be distinguishable from any other invalid token.
func unauthenticated(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_token", "invalid or expired access token", cause)
}

// Verify parses, cryptographically verifies and validates tokenString,
// returning its Claims on success.
func (v *Verifier) Verify(tokenString string) (Claims, error) {
	claims := &Claims{}

	keyFunc := func(t *jwt.Token) (any, error) {
		// Fix the expected algorithm explicitly -- never trust the
		// token's own header to select the verification method.
		method, ok := t.Method.(*jwt.SigningMethodHMAC)
		if !ok || method != signingMethod {
			return nil, errUnexpectedSigningMethod
		}

		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errUnknownKeyID
		}
		secret, ok := v.keys[kid]
		if !ok {
			return nil, errUnknownKeyID
		}
		return secret, nil
	}

	_, err := jwt.ParseWithClaims(tokenString, claims, keyFunc,
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithLeeway(clockSkewLeeway),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Claims{}, unauthenticated(err)
	}

	if claims.TenantID == "" {
		return Claims{}, unauthenticated(errors.New("tenant_id claim is missing or empty"))
	}
	if !claims.Role.Valid() {
		return Claims{}, unauthenticated(fmt.Errorf("role claim %q is not a known role", claims.Role))
	}
	if claims.SessionID == "" {
		return Claims{}, unauthenticated(errors.New("sid claim is missing or empty"))
	}

	return *claims, nil
}
