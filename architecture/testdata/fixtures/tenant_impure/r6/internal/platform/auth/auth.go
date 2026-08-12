// Package auth is a fixture standing in for the real
// internal/platform/auth package -- the one place (besides _test.go files)
// allowed to call tenant.Parse directly (R6).
package auth

import "fixture/tenantimpure/r6/internal/platform/tenant"

// Authenticate calls tenant.Parse from an allowed location. R6 must NOT
// flag this reference.
func Authenticate(raw string) (string, error) {
	return tenant.Parse(raw)
}
