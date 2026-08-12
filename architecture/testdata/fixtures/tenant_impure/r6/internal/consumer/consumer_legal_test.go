package consumer

import (
	"testing"

	"fixture/tenantimpure/r6/internal/platform/tenant"
)

// TestLegalReferenceFromTestFile calls tenant.Parse directly from a
// _test.go file. R6 must NOT flag this reference -- test files are exempt.
func TestLegalReferenceFromTestFile(t *testing.T) {
	t.Helper()
	if _, err := tenant.Parse("abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
