// Package c is a fixture module package that imports module b's contracts
// package -- the ALLOWED way to depend on another module. R3 must NOT flag
// this package.
package c

import "fixture/crossmodule/internal/modules/b/contracts"

// Secret re-exports b's public contract value.
var Secret = contracts.Secret
