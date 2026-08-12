package token

import "github.com/golang-jwt/jwt/v5"

// Role identifies a user's role within a tenant, drawn from a closed set
// of known values. The zero value ("") is deliberately invalid: Valid()
// returns false for it, and Verify rejects any token carrying an unknown
// or empty role instead of letting it flow through as a silent default.
type Role string

const (
	// RoleGestor is a tenant's management-level role.
	RoleGestor Role = "gestor"
	// RoleFuncionario is a tenant's staff-level role.
	RoleFuncionario Role = "funcionario"
)

// Valid reports whether r is one of the closed set of known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleGestor, RoleFuncionario:
		return true
	default:
		return false
	}
}

// Claims is the access-token claim set issued and validated by this
// package.
//
// TenantID is intentionally a raw string, not a tenant.TenantID: it has
// not yet passed tenant.Parse's validation. That validation happens later,
// in the authentication middleware (Bloco D) that consumes an
// already-verified Claims value -- this package only checks the claim is
// present and non-empty, deliberately not calling tenant.Parse itself
// (per architecture rule R6, tenant.Parse is reserved for
// internal/platform/auth/** production code that actually needs it, and
// this package has no need to produce a real tenant.TenantID).
//
// Subject (the JWT "sub" claim) is provided via the embedded
// jwt.RegisteredClaims, which also supplies exp/nbf/iat/iss/aud/jti.
type Claims struct {
	// TenantID is the tenant_id claim, unvalidated beyond JSON shape.
	TenantID string `json:"tenant_id"`
	// Role is the caller's role within TenantID.
	Role Role `json:"role"`
	// SessionID links this access token to the refresh-token family that
	// authorized it (see internal/platform/auth/refresh).
	SessionID string `json:"sid"`

	jwt.RegisteredClaims
}
