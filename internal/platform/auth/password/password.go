// Package password implements secure password hashing and verification for
// mesaposta-api using Argon2id, via github.com/alexedwards/argon2id -- a
// thin, idiomatic wrapper around golang.org/x/crypto/argon2 that stores the
// cost parameters and salt inside the PHC-encoded hash string itself, so
// they never need to be tracked separately.
package password

import (
	"errors"
	"fmt"

	"github.com/alexedwards/argon2id"
)

// currentParams are the Argon2id cost parameters used to hash new
// passwords. They also define the security floor: any encoded hash whose
// embedded parameters fall below these values is flagged by NeedsRehash.
//
// Do NOT lower these values without an explicit, reviewed security
// decision -- doing so weakens password storage for every account whose
// hash gets rehashed after the change. password_test.go
// (TestCurrentParamsMeetSecurityFloor) pins these to a hard minimum and
// fails the build if they regress.
var currentParams = &argon2id.Params{
	Memory:      64 * 1024, // KiB (64 MiB)
	Iterations:  2,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// minParams is the floor NeedsRehash compares an existing hash's embedded
// parameters against. It intentionally omits Parallelism: changing the
// thread count changes the hash output but, unlike memory/time/salt/key
// size, does not by itself represent a "weaker" hash worth forcing a
// rehash over.
var minParams = struct {
	memory     uint32
	iterations uint32
	saltLength uint32
	keyLength  uint32
}{
	memory:     currentParams.Memory,
	iterations: currentParams.Iterations,
	saltLength: currentParams.SaltLength,
	keyLength:  currentParams.KeyLength,
}

// Hash returns an Argon2id PHC-encoded hash of password, generated with a
// fresh, cryptographically random salt and the package's currentParams --
// so hashing the same password twice always yields two different encoded
// strings.
func Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password: password must not be empty")
	}

	hash, err := argon2id.CreateHash(password, currentParams)
	if err != nil {
		return "", fmt.Errorf("password: hashing: %w", err)
	}
	return hash, nil
}

// Verify reports whether password matches encodedHash, using the
// parameters and salt embedded in encodedHash itself (via a constant-time
// comparison performed by argon2id.ComparePasswordAndHash).
//
// Verify returns an error -- and always false -- if encodedHash is empty
// or is not a well-formed Argon2id PHC-encoded hash (e.g. corrupted,
// truncated, produced by an incompatible algorithm/variant/version). It
// never returns (true, non-nil error): a non-nil error always implies
// match == false.
func Verify(password, encodedHash string) (bool, error) {
	if encodedHash == "" {
		return false, errors.New("password: encoded hash must not be empty")
	}

	match, err := argon2id.ComparePasswordAndHash(password, encodedHash)
	if err != nil {
		return false, fmt.Errorf("password: verifying: %w", err)
	}
	return match, nil
}

// NeedsRehash reports whether encodedHash was produced with Argon2id
// parameters weaker than the package's current floor (see currentParams)
// and should therefore be regenerated the next time its plaintext password
// is available to the caller -- typically right after a successful Verify
// during login.
func NeedsRehash(encodedHash string) (bool, error) {
	if encodedHash == "" {
		return false, errors.New("password: encoded hash must not be empty")
	}

	params, _, _, err := argon2id.DecodeHash(encodedHash)
	if err != nil {
		return false, fmt.Errorf("password: decoding hash: %w", err)
	}

	return belowFloor(params), nil
}

// belowFloor reports whether params falls short of minParams on any of the
// dimensions that matter for password-hash strength.
func belowFloor(params *argon2id.Params) bool {
	return params.Memory < minParams.memory ||
		params.Iterations < minParams.iterations ||
		params.SaltLength < minParams.saltLength ||
		params.KeyLength < minParams.keyLength
}
