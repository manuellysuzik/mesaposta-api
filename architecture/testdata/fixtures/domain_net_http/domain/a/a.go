// Package a is a fixture domain package that does NOT import net/http
// directly, but transitively reaches it through package b. It exists to
// prove R1's transitive closure check: R1 must flag package a even though
// only package b has the direct import.
package a

import "fixture/domainnethttp/domain/b"

// Status returns the HTTP status code exposed by package b.
func Status() int {
	return b.StatusOK()
}
