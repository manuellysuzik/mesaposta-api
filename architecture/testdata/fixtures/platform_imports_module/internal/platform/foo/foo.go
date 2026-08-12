// Package foo is a fixture internal/platform package that imports
// internal/modules/bar, violating R2 (internal/platform must not import
// internal/modules).
package foo

import "fixture/platformimportsmodule/internal/modules/bar"

// Value re-exports bar.Value so the import above is not unused.
var Value = bar.Value
