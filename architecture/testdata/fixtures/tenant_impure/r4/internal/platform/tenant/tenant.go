// Package tenant is a fixture standing in for the real
// internal/platform/tenant package. It imports a (deliberately
// unresolvable) third-party package, violating R4 (internal/platform/errs
// and internal/platform/tenant may only import the standard library).
//
// This fixture module is intentionally left uncompileable: R4 only needs
// the import graph (packages.Load with NeedImports|NeedDeps), which does
// not require the imported package to actually resolve or type-check. See
// architecture/arch_test.go for the mode used to load this fixture.
package tenant

import "github.com/some/thirdparty"

// Parse stands in for the real tenant.Parse function.
func Parse(raw string) (thirdparty.ID, error) {
	return thirdparty.Parse(raw)
}
