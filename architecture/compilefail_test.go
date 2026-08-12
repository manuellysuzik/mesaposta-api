package architecture

import (
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// poolQueryUndefinedPattern matches the Go compiler's "no such
// method/field" diagnostic for a call like pool.Query(...) where pool's
// type has no Query method -- e.g. "pool.Query undefined (type *db.Pool
// has no field or method Query)". It is intentionally loose about exact
// wording (which can drift across Go versions) while still requiring the
// diagnostic to be about "pool" and about a missing member.
var poolQueryUndefinedPattern = regexp.MustCompile(`(?i)pool.*(undefined|has no field or method)`)

// TestPoolHasNoQueryMethod is the compile-time proof underpinning this
// block's core acceptance criterion: it is not merely a convention that
// nothing outside internal/platform/db can run a SQL query without going
// through a tenant-scoped db.Scope -- it is IMPOSSIBLE to compile code
// that tries. architecture/testdata/fixtures/queryless_pool reproduces the
// real db.Pool/db.Scope split (Pool has no Query/Exec/QueryRow; only Scope
// does) and contains exactly one file, internal/misuse/misuse.go, that
// tries to call pool.Query(...) directly on a *db.Pool. This test loads
// that fixture via packages.Load (which invokes the real Go compiler's
// type checker) and asserts it fails to compile, with a diagnostic that
// specifically names the missing method -- not just "the fixture module
// failed to load for some unrelated reason".
func TestPoolHasNoQueryMethod(t *testing.T) {
	mode := packages.NeedName | packages.NeedImports | packages.NeedDeps |
		packages.NeedSyntax | packages.NeedTypes

	pkgs := loadFixture(t, "queryless_pool", mode, false)

	// packages.Load, for a module with a compile error, surfaces the
	// failure on the specific package(s) that fail to type-check
	// (internal/misuse here); walk every loaded package's own Errors
	// field to collect them all.
	var allErrors []packages.Error
	for _, pkg := range pkgs {
		allErrors = append(allErrors, pkg.Errors...)
	}

	if len(allErrors) == 0 {
		t.Fatal("expected the queryless_pool fixture to fail to compile (pool.Query(...) called on a *db.Pool, " +
			"which has no Query method), but packages.Load reported zero errors -- db.Pool may have accidentally " +
			"gained a Query/Exec/QueryRow method, or the fixture no longer actually exercises the misuse it claims to")
	}

	var matched bool
	var messages []string
	for _, e := range allErrors {
		messages = append(messages, e.Msg)
		if poolQueryUndefinedPattern.MatchString(e.Msg) {
			matched = true
		}
	}

	if !matched {
		t.Fatalf("queryless_pool fixture failed to compile (good), but no error matched the expected "+
			"\"pool ... undefined / has no field or method\" pattern; got %d error(s): %s",
			len(allErrors), strings.Join(messages, " | "))
	}
}
