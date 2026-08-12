// Package a is a fixture module package that imports module b's internal
// package directly, violating R3 (cross-module imports must go through
// <module>/contracts).
package a

import "fixture/crossmodule/internal/modules/b/internal"

// Secret re-exports b's internal secret so the illegal import above is not
// unused.
var Secret = internal.Secret
