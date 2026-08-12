// Package main is a fixture cmd/** package that imports net/http directly.
// It exists to prove R5: cmd/** is exempt from R1 (and every other rule
// that targets domain/platform/module packages) -- the fitness function
// must NOT flag this package.
package main

import "net/http"

func main() {
	_ = http.StatusOK
}
