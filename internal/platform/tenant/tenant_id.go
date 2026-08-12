// Package tenant provides the mesaposta-api multi-tenancy primitive:
// TenantID, a validated 16-byte tenant identifier, and the context
// plumbing (With/From) used to carry it through a request's lifetime.
//
// TenantID is intentionally hard to forge: its only field is unexported,
// so the sole way to obtain a non-zero value from outside this package is
// Parse, which validates its input as a well-formed, non-nil UUID. Code
// elsewhere in the codebase (in particular, request handlers) must never
// be able to manufacture a TenantID directly from untrusted input --
// tenant identity is meant to flow exclusively through an authenticated
// middleware that calls Parse once and then propagates the result via
// With/From (see context.go).
//
// This package is part of the shared kernel every layer -- including
// internal/domain/** -- is allowed to depend on (see
// architecture/rules.go, R1's platform allowlist). Because of that, and
// per R4, it must only ever import the standard library plus
// internal/platform/errs: no other third-party dependency may leak into
// it, including UUID-generation libraries.
package tenant

import (
	"encoding/hex"
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// TenantID identifies a tenant. The zero value is invalid; obtain a real
// TenantID via Parse. The underlying field is unexported so that a
// TenantID cannot be forged via a composite literal from outside this
// package.
type TenantID struct {
	v [16]byte
}

// uuidTextLen is the length of the canonical UUID text form:
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
const uuidTextLen = 36

// dashPositions are the indexes within the canonical UUID text form that
// must hold a '-' separator.
var dashPositions = [4]int{8, 13, 18, 23}

// Parse validates s as a canonical, non-nil UUID string and returns the
// corresponding TenantID. It performs its own hand-written validation --
// deliberately not importing any third-party UUID library, to keep this
// package 100% standard-library (see architecture rule R4).
//
// Parse rejects: the empty string, any string that isn't a well-formed
// UUID (wrong length, missing/misplaced dashes, non-hex characters), and
// the nil UUID ("00000000-0000-0000-0000-000000000000"). All rejections
// are reported as an *errs.Error with Kind errs.Invalid.
func Parse(s string) (TenantID, error) {
	if err := validateUUIDFormat(s); err != nil {
		return TenantID{}, err
	}

	hexDigits := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	raw, err := hex.DecodeString(hexDigits)
	if err != nil {
		return TenantID{}, errs.New(errs.Invalid, "invalid_tenant_id",
			"tenant id must be a valid UUID", fmt.Errorf("decoding hex digits of %q: %w", s, err))
	}

	var id TenantID
	copy(id.v[:], raw)

	if id.IsZero() {
		return TenantID{}, errs.New(errs.Invalid, "invalid_tenant_id",
			"tenant id must not be the nil UUID", fmt.Errorf("%q is the nil UUID", s))
	}

	return id, nil
}

// validateUUIDFormat checks s has the canonical UUID shape (36 characters,
// dashes at the 4 fixed positions, hex digits everywhere else) without
// yet checking for the nil UUID.
func validateUUIDFormat(s string) error {
	if len(s) != uuidTextLen {
		return errs.New(errs.Invalid, "invalid_tenant_id", "tenant id must be a valid UUID",
			fmt.Errorf("expected %d characters, got %d for %q", uuidTextLen, len(s), s))
	}

	for i := 0; i < len(s); i++ {
		if isDashPosition(i) {
			if s[i] != '-' {
				return errs.New(errs.Invalid, "invalid_tenant_id", "tenant id must be a valid UUID",
					fmt.Errorf("expected '-' at position %d in %q", i, s))
			}
			continue
		}
		if !isHexDigit(s[i]) {
			return errs.New(errs.Invalid, "invalid_tenant_id", "tenant id must be a valid UUID",
				fmt.Errorf("invalid hex digit %q at position %d in %q", s[i], i, s))
		}
	}

	return nil
}

// isDashPosition reports whether i is one of the fixed dash positions in
// the canonical UUID text form.
func isDashPosition(i int) bool {
	for _, p := range dashPositions {
		if i == p {
			return true
		}
	}
	return false
}

// isHexDigit reports whether b is an ASCII hex digit (0-9, a-f, A-F).
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// IsZero reports whether id is the zero value (i.e. was never populated by
// Parse).
func (id TenantID) IsZero() bool {
	return id.v == [16]byte{}
}

// String renders id in canonical UUID text form. A zero-value TenantID
// renders as "<invalid>" rather than an empty string -- an empty string
// returned silently from String() is dangerous if it ever ends up
// interpolated into a log line or, worse, a SQL query.
func (id TenantID) String() string {
	if id.IsZero() {
		return "<invalid>"
	}
	h := hex.EncodeToString(id.v[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
