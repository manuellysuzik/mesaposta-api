package password_test

import (
	"testing"

	"github.com/alexedwards/argon2id"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
)

func TestHash_SamePasswordProducesDifferentHashes(t *testing.T) {
	const pw = "correct horse battery staple"

	hash1, err := password.Hash(pw)
	if err != nil {
		t.Fatalf("Hash() #1 returned error: %v", err)
	}
	hash2, err := password.Hash(pw)
	if err != nil {
		t.Fatalf("Hash() #2 returned error: %v", err)
	}

	if hash1 == hash2 {
		t.Fatal("Hash() returned identical hashes for two calls with the same password; salt must be random")
	}
}

func TestHash_RejectsEmptyPassword(t *testing.T) {
	if _, err := password.Hash(""); err == nil {
		t.Fatal(`Hash("") expected error, got nil`)
	}
}

func TestVerify_CorrectPasswordMatches(t *testing.T) {
	const pw = "s3cr3t-p@ssword!"

	hash, err := password.Hash(pw)
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}

	match, err := password.Verify(pw, hash)
	if err != nil {
		t.Fatalf("Verify() returned error: %v", err)
	}
	if !match {
		t.Fatal("Verify() = false for the correct password, want true")
	}
}

func TestVerify_WrongPasswordDoesNotMatchAndReturnsNoError(t *testing.T) {
	hash, err := password.Hash("correct-password")
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}

	match, err := password.Verify("wrong-password", hash)
	if err != nil {
		t.Fatalf("Verify() returned unexpected error for a wrong password: %v", err)
	}
	if match {
		t.Fatal("Verify() = true for a wrong password, want false")
	}
}

func TestVerify_EmptyEncodedHashReturnsErrorNeverTrue(t *testing.T) {
	match, err := password.Verify("anything", "")
	if err == nil {
		t.Fatal("Verify() with empty encoded hash: expected error, got nil")
	}
	if match {
		t.Fatal("Verify() with empty encoded hash returned match=true; must never be true on error")
	}
}

func TestVerify_MalformedHashReturnsErrorNeverTrue(t *testing.T) {
	match, err := password.Verify("anything", "not-a-valid-phc-hash")
	if err == nil {
		t.Fatal("Verify() with malformed hash: expected error, got nil")
	}
	if match {
		t.Fatal("Verify() with malformed hash returned match=true; must never be true on error")
	}
}

func TestVerify_TamperedHashKeyBytesNeverMatches(t *testing.T) {
	hash, err := password.Hash("tamper-target-password")
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}

	// Flip the last character of the encoded key -- still a well-formed PHC
	// string (same shape, same length), just wrong key bytes.
	tampered := hash[:len(hash)-1]
	if hash[len(hash)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}

	match, err := password.Verify("tamper-target-password", tampered)
	if match {
		t.Fatalf("Verify() with tampered hash returned match=true (err=%v); must never be true", err)
	}
}

func TestNeedsRehash_HashProducedByCurrentParamsDoesNotNeedRehash(t *testing.T) {
	hash, err := password.Hash("some-password")
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}

	needs, err := password.NeedsRehash(hash)
	if err != nil {
		t.Fatalf("NeedsRehash() returned error: %v", err)
	}
	if needs {
		t.Fatal("NeedsRehash() = true for a hash produced with current parameters, want false")
	}
}

func TestNeedsRehash_HashBelowCurrentFloorNeedsRehash(t *testing.T) {
	weakParams := &argon2id.Params{
		Memory:      8 * 1024, // 8 MiB -- deliberately below the 64 MiB floor
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
	weakHash, err := argon2id.CreateHash("some-password", weakParams)
	if err != nil {
		t.Fatalf("argon2id.CreateHash() returned error: %v", err)
	}

	needs, err := password.NeedsRehash(weakHash)
	if err != nil {
		t.Fatalf("NeedsRehash() returned error: %v", err)
	}
	if !needs {
		t.Fatal("NeedsRehash() = false for a hash below the current parameter floor, want true")
	}
}

func TestNeedsRehash_EmptyEncodedHashReturnsError(t *testing.T) {
	if _, err := password.NeedsRehash(""); err == nil {
		t.Fatal(`NeedsRehash("") expected error, got nil`)
	}
}

func TestNeedsRehash_MalformedEncodedHashReturnsError(t *testing.T) {
	if _, err := password.NeedsRehash("not-a-valid-phc-hash"); err == nil {
		t.Fatal("NeedsRehash() with malformed hash: expected error, got nil")
	}
}

// TestCurrentParamsMeetSecurityFloor guards against silently lowering the
// Argon2id cost parameters below the project's minimum security floor. It
// hashes a password with the package's *actual* current parameters and
// decodes the resulting PHC string -- so it fails for real if anyone
// weakens the parameters, even if they rename or restructure the internal
// variables that hold them.
func TestCurrentParamsMeetSecurityFloor(t *testing.T) {
	hash, err := password.Hash("floor-check-password")
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}

	params, _, _, err := argon2id.DecodeHash(hash)
	if err != nil {
		t.Fatalf("argon2id.DecodeHash() returned error: %v", err)
	}

	const (
		minMemory     uint32 = 64 * 1024 // 64 MiB, expressed in KiB
		minIterations uint32 = 2
		minSaltLength uint32 = 16
		minKeyLength  uint32 = 32
	)

	if params.Memory < minMemory {
		t.Errorf("Memory = %d KiB, want >= %d KiB (64 MiB); lowering this weakens password storage for every account", params.Memory, minMemory)
	}
	if params.Iterations < minIterations {
		t.Errorf("Iterations (time cost) = %d, want >= %d", params.Iterations, minIterations)
	}
	if params.Parallelism < 1 {
		t.Errorf("Parallelism = %d, want >= 1", params.Parallelism)
	}
	if params.SaltLength < minSaltLength {
		t.Errorf("SaltLength = %d, want >= %d", params.SaltLength, minSaltLength)
	}
	if params.KeyLength < minKeyLength {
		t.Errorf("KeyLength = %d, want >= %d", params.KeyLength, minKeyLength)
	}
}
