// Package tenant is a fixture standing in for the real
// internal/platform/tenant package. Unlike r4's fixture, this one is
// stdlib-clean and fully type-checkable, because R6 needs
// NeedSyntax|NeedTypesInfo to resolve symbol references to tenant.Parse,
// and an unresolved import elsewhere in the package would poison
// type-checking for the whole package.
package tenant

import "errors"

// ErrEmpty is returned by Parse when raw is empty.
var ErrEmpty = errors.New("tenant: empty id")

// Parse stands in for the real tenant.Parse function. Only
// internal/platform/auth/** and _test.go files are allowed to reference it
// directly (R6); every other caller must go through the auth layer.
func Parse(raw string) (string, error) {
	if raw == "" {
		return "", ErrEmpty
	}
	return raw, nil
}
