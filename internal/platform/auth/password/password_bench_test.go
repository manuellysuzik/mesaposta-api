package password_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
)

// BenchmarkHash documents the wall-clock cost of Hash with the package's
// current Argon2id parameters (64 MiB memory, 2 iterations, 4 threads).
// The target is roughly 50-100ms/op -- comfortably slow enough to blunt
// offline brute-force attempts, comfortably fast enough for interactive
// login. This benchmark is not wired into `make check` and never fails
// CI; run it explicitly when tuning cost parameters:
//
//	go test -bench=Hash -benchtime=5x ./internal/platform/auth/password/...
func BenchmarkHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := password.Hash("benchmark-password-42"); err != nil {
			b.Fatalf("Hash() returned error: %v", err)
		}
	}
}

// BenchmarkVerify documents the wall-clock cost of Verify, which performs
// the same Argon2id computation as Hash (using the parameters embedded in
// the hash being checked) plus a constant-time comparison.
func BenchmarkVerify(b *testing.B) {
	hash, err := password.Hash("benchmark-password-42")
	if err != nil {
		b.Fatalf("Hash() returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := password.Verify("benchmark-password-42", hash); err != nil {
			b.Fatalf("Verify() returned error: %v", err)
		}
	}
}
