// Package consumer is a fixture package that is neither
// internal/platform/auth/** nor a _test.go file, yet references
// tenant.Parse directly -- violating R6.
package consumer

import "fixture/tenantimpure/r6/internal/platform/tenant"

// Use calls tenant.Parse from a location that is not allowed to.
func Use(raw string) (string, error) {
	return tenant.Parse(raw)
}
