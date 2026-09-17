// Package domain implements the identity module's domain layer: the User
// aggregate, its closed-set Role type, and the repository/unit-of-work
// ports the application layer (../application) depends on.
//
// Per architecture rule R1 (see internal/modules/README.md), this package
// may only import the standard library plus
// internal/platform/{errs,tenant,audit} -- notably NOT
// internal/platform/auth/token, even though that package also defines a
// Role type with the exact same two values ("manager"/"employee"):
// token.Role lives under internal/platform/auth, which transitively pulls
// in github.com/golang-jwt/jwt, one of R1's explicitly forbidden imports.
// This package therefore defines its OWN closed Role type (enums.go),
// structurally identical but a distinct Go type; converting between the two
// (domain.Role -> token.Role) happens ONLY in
// ../infrastructure/http, the one layer of this module allowed to know JWTs
// exist at all.
package domain
