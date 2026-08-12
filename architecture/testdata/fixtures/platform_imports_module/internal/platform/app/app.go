// Package app is a fixture standing in for mesaposta-api's real
// internal/platform/app composition root. It imports internal/modules/bar,
// exactly like package foo (the sibling fixture) does -- the difference
// this fixture package exists to prove is that R2 must NOT flag this one:
// per R5 (see internal/modules/README.md and the real
// internal/platform/app.NewApp doc comment), the composition root is
// exempt from "platform must not import modules" because wiring modules
// together is its entire purpose.
package app

import "fixture/platformimportsmodule/internal/modules/bar"

// Value re-exports bar.Value so the import above is not unused.
var Value = bar.Value
