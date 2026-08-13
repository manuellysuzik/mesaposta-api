package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// ID identifies an entity within the ordering module (a table, an order,
// ...).
//
// Like internal/platform/tenant.TenantID and every other module's own ID
// type in this codebase, ID is a struct with an unexported field rather
// than a plain string: this makes it impossible to forge a non-zero ID
// from outside this package via a composite literal or a raw string cast --
// the only ways to obtain a real ID are NewID (a freshly generated UUID v4)
// and ParseID (validating a UUID string read back from, e.g., Postgres or
// an HTTP request).
//
// This type is deliberately NOT shared with any other module's own ID type,
// even though the implementation is byte-for-byte identical: architecture
// rule R3 forbids one module's domain/ from importing another module's
// domain/ at all (only <module>/contracts is reachable across modules), so
// each module owns its own copy of this small, self-contained primitive
// rather than introducing a shared-kernel ID package that would blur module
// boundaries.
type ID struct {
	v [16]byte
}

// idTextLen is the length of the canonical UUID text form:
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
const idTextLen = 36

// idDashPositions are the indexes within the canonical UUID text form that
// must hold a '-' separator.
var idDashPositions = [4]int{8, 13, 18, 23}

// NewID generates a new random UUID v4 (RFC 4122 section 4.4) ID, using
// crypto/rand as the sole source of randomness (no third-party
// UUID-generation library is used or needed, per architecture rule R4).
//
// crypto/rand.Read only fails when the operating system's CSPRNG cannot be
// read at all -- an unrecoverable environment failure that no caller could
// meaningfully recover from -- so NewID panics rather than threading an
// error return through every call site, consistent with how the Go
// standard library itself treats a broken crypto/rand source as fatal.
func NewID() ID {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("domain: NewID: crypto/rand.Read failed: %v", err))
	}

	// Per RFC 4122 §4.4: set the 4 version bits to 0100 (version 4) and the
	// 2 most-significant variant bits to 10 (the RFC 4122 variant).
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	return ID{v: raw}
}

// ParseID validates s as a canonical, non-nil UUID string and returns the
// corresponding ID. It performs its own hand-written validation --
// deliberately not importing any third-party UUID library, to keep this
// package free of non-stdlib dependencies (architecture rule R1/R4).
//
// ParseID rejects: the empty string, any string that isn't a well-formed
// UUID (wrong length, missing/misplaced dashes, non-hex characters), and
// the nil UUID ("00000000-0000-0000-0000-000000000000"). All rejections
// are reported as an *errs.Error with Kind errs.Invalid.
func ParseID(s string) (ID, error) {
	if err := validateIDFormat(s); err != nil {
		return ID{}, err
	}

	hexDigits := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	raw, err := hex.DecodeString(hexDigits)
	if err != nil {
		return ID{}, errs.New(errs.Invalid, "invalid_id", "id must be a valid UUID",
			fmt.Errorf("decoding hex digits of %q: %w", s, err))
	}

	var id ID
	copy(id.v[:], raw)

	if id.IsZero() {
		return ID{}, errs.New(errs.Invalid, "invalid_id", "id must not be the nil UUID",
			fmt.Errorf("%q is the nil UUID", s))
	}

	return id, nil
}

// validateIDFormat checks s has the canonical UUID shape (36 characters,
// dashes at the 4 fixed positions, hex digits everywhere else) without yet
// checking for the nil UUID.
func validateIDFormat(s string) error {
	if len(s) != idTextLen {
		return errs.New(errs.Invalid, "invalid_id", "id must be a valid UUID",
			fmt.Errorf("expected %d characters, got %d for %q", idTextLen, len(s), s))
	}

	for i := 0; i < len(s); i++ {
		if isIDDashPosition(i) {
			if s[i] != '-' {
				return errs.New(errs.Invalid, "invalid_id", "id must be a valid UUID",
					fmt.Errorf("expected '-' at position %d in %q", i, s))
			}
			continue
		}
		if !isIDHexDigit(s[i]) {
			return errs.New(errs.Invalid, "invalid_id", "id must be a valid UUID",
				fmt.Errorf("invalid hex digit %q at position %d in %q", s[i], i, s))
		}
	}

	return nil
}

// isIDDashPosition reports whether i is one of the fixed dash positions in
// the canonical UUID text form.
func isIDDashPosition(i int) bool {
	for _, p := range idDashPositions {
		if i == p {
			return true
		}
	}
	return false
}

// isIDHexDigit reports whether b is an ASCII hex digit (0-9, a-f, A-F).
func isIDHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// IsZero reports whether id is the zero value (i.e. was never populated by
// NewID or ParseID).
func (id ID) IsZero() bool {
	return id.v == [16]byte{}
}

// String renders id in canonical UUID text form. A zero-value ID renders
// as "<invalid>" rather than an empty string -- a silently empty string is
// dangerous if it ever ends up interpolated into a log line or, worse, a
// SQL query.
func (id ID) String() string {
	if id.IsZero() {
		return "<invalid>"
	}
	h := hex.EncodeToString(id.v[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
