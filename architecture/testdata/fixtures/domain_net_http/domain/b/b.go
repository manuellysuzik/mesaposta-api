// Package b is a fixture domain package that imports net/http directly,
// violating R1 (domain packages must not reach net/http).
package b

import "net/http"

// StatusOK exposes http.StatusOK so the import above is not unused.
func StatusOK() int {
	return http.StatusOK
}
