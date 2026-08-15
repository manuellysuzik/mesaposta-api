package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// Role identifies a User's role within their tenant, drawn from a closed
// set of known values. The zero value ("") is deliberately invalid: Valid
// returns false for it, and ParseRole rejects it just like any other
// unknown value.
//
// Role is structurally identical to internal/platform/auth/token.Role, but
// deliberately a DISTINCT Go type: this package's own doc comment (doc.go)
// explains why domain/ cannot import token.Role directly (architecture rule
// R1). Converting a domain.Role to a token.Role -- e.g. right before
// calling token.Issuer.Issue -- is ../infrastructure/http's job, never
// this package's or ../application's.
type Role string

const (
	// RoleGestor is a tenant's management-level role: can invite/manage
	// staff (see ../application/create_user.go) in addition to everything
	// RoleFuncionario can do.
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

// ParseRole validates s against the closed set of known roles.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !r.Valid() {
		return "", errs.New(errs.Invalid, "invalid_role", "unknown user role",
			fmt.Errorf("role %q is not one of the known values", s))
	}
	return r, nil
}
