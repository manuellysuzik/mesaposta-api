package errs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestKindOf_NilError_ReturnsKindNone(t *testing.T) {
	// Arrange
	var err error

	// Act
	got := errs.KindOf(err)

	// Assert
	if got != errs.KindNone {
		t.Errorf("KindOf(nil) = %v, want %v", got, errs.KindNone)
	}
}

func TestKindOf_RawError_ClassifiesAsInternal(t *testing.T) {
	// Arrange
	err := errors.New("boom")

	// Act
	got := errs.KindOf(err)

	// Assert
	if got != errs.Internal {
		t.Errorf("KindOf(raw error) = %v, want %v", got, errs.Internal)
	}
}

func TestKindOf_DirectStructuredError_ReturnsItsKind(t *testing.T) {
	// Arrange
	err := errs.New(errs.NotFound, "market_not_found", "market not found", nil)

	// Act
	got := errs.KindOf(err)

	// Assert
	if got != errs.NotFound {
		t.Errorf("KindOf(err) = %v, want %v", got, errs.NotFound)
	}
}

func TestKindOf_TraversesFmtErrorfWrapChain(t *testing.T) {
	// Arrange
	base := errs.New(errs.Conflict, "duplicate_bet", "this bet already exists", errors.New("unique constraint violated"))
	wrapped := fmt.Errorf("placing bet: %w", base)
	doubleWrapped := fmt.Errorf("handling request: %w", wrapped)

	// Act
	got := errs.KindOf(doubleWrapped)

	// Assert
	if got != errs.Conflict {
		t.Errorf("KindOf(doubly-wrapped) = %v, want %v", got, errs.Conflict)
	}
}

func TestIsKind_TraversesWrapChainAndMatches(t *testing.T) {
	// Arrange
	base := errs.New(errs.Forbidden, "not_allowed", "you cannot do that", nil)
	wrapped := fmt.Errorf("checking permission: %w", base)

	// Act & Assert
	if !errs.IsKind(wrapped, errs.Forbidden) {
		t.Errorf("IsKind(wrapped, Forbidden) = false, want true")
	}
	if errs.IsKind(wrapped, errs.NotFound) {
		t.Errorf("IsKind(wrapped, NotFound) = true, want false")
	}
}

func TestIsKind_NilError_NeverMatchesAnyKindExceptNone(t *testing.T) {
	// Arrange
	var err error

	// Act & Assert
	if !errs.IsKind(err, errs.KindNone) {
		t.Errorf("IsKind(nil, KindNone) = false, want true")
	}
	if errs.IsKind(err, errs.Internal) {
		t.Errorf("IsKind(nil, Internal) = true, want false")
	}
}

func TestNew_AllKindConstants_AreDistinct(t *testing.T) {
	// Arrange
	kinds := []errs.Kind{
		errs.KindNone,
		errs.Invalid,
		errs.Unauthenticated,
		errs.Forbidden,
		errs.NotFound,
		errs.Conflict,
		errs.TooManyRequests,
		errs.Internal,
	}

	// Act & Assert
	seen := map[errs.Kind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate Kind value found: %v", k)
		}
		seen[k] = true
	}
	if len(seen) != len(kinds) {
		t.Fatalf("expected %d distinct kinds, got %d", len(kinds), len(seen))
	}
}

func TestKindNone_IsZeroValue(t *testing.T) {
	// Arrange
	var k errs.Kind

	// Act & Assert
	if k != errs.KindNone {
		t.Errorf("zero value of Kind = %v, want KindNone", k)
	}
}

func TestError_Kind_ReturnsConstructedKind(t *testing.T) {
	// Arrange
	err := errs.New(errs.Invalid, "invalid_argument", "the request is invalid", nil)

	// Act & Assert
	if err.Kind() != errs.Invalid {
		t.Errorf("err.Kind() = %v, want %v", err.Kind(), errs.Invalid)
	}
}

func TestError_Code_ReturnsStableCode(t *testing.T) {
	// Arrange
	err := errs.New(errs.Invalid, "invalid_argument", "the request is invalid", nil)

	// Act & Assert
	if err.Code() != "invalid_argument" {
		t.Errorf("err.Code() = %q, want %q", err.Code(), "invalid_argument")
	}
}

func TestError_Public_NeverLeaksInternalCause(t *testing.T) {
	// Arrange
	const sensitive = "senha=segredo123"
	internalCause := fmt.Errorf("db connection failed: %s", sensitive)
	err := errs.New(errs.Internal, "internal_error", "something went wrong, please try again", internalCause)

	// Act
	public := err.Public()

	// Assert
	if strings.Contains(public, sensitive) {
		t.Fatalf("Public() leaked sensitive internal cause: %q", public)
	}
	if public != "something went wrong, please try again" {
		t.Errorf("Public() = %q, want the exact configured public message", public)
	}
}

func TestError_Error_IncludesCauseForLogging(t *testing.T) {
	// Arrange
	const sensitive = "senha=segredo123"
	internalCause := errors.New(sensitive)
	err := errs.New(errs.Internal, "internal_error", "something went wrong", internalCause)

	// Act
	full := err.Error()

	// Assert -- unlike Public(), the Error() string (meant for server-side
	// logs, never sent to clients) MUST retain the underlying cause so
	// operators can debug the failure.
	if !strings.Contains(full, sensitive) {
		t.Errorf("Error() = %q, want it to contain the wrapped cause for logging purposes", full)
	}
}

func TestError_Unwrap_ExposesCauseToErrorsIs(t *testing.T) {
	// Arrange
	sentinel := errors.New("sentinel cause")
	err := errs.New(errs.Internal, "internal_error", "something went wrong", sentinel)

	// Act & Assert
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false, want true (Unwrap must expose the cause)")
	}
}

func TestError_Unwrap_NilCause_DoesNotPanic(t *testing.T) {
	// Arrange
	err := errs.New(errs.NotFound, "not_found", "resource not found", nil)

	// Act & Assert -- must not panic, and must not accidentally unwrap to a
	// non-nil error.
	if errors.Unwrap(err) != nil {
		t.Errorf("errors.Unwrap(err) = %v, want nil when constructed without a cause", errors.Unwrap(err))
	}
}

func TestKind_String_RendersKnownNames(t *testing.T) {
	// Arrange
	tests := []struct {
		kind errs.Kind
		want string
	}{
		{errs.KindNone, "none"},
		{errs.Invalid, "invalid"},
		{errs.Unauthenticated, "unauthenticated"},
		{errs.Forbidden, "forbidden"},
		{errs.NotFound, "not_found"},
		{errs.Conflict, "conflict"},
		{errs.TooManyRequests, "too_many_requests"},
		{errs.Internal, "internal"},
		{errs.Kind(999), "unknown"},
	}

	for _, tt := range tests {
		// Act
		got := tt.kind.String()

		// Assert
		if got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestError_ImplementsErrorInterface(t *testing.T) {
	// Arrange & Act
	var err error = errs.New(errs.Invalid, "invalid_argument", "bad request", nil)

	// Assert
	if err.Error() == "" {
		t.Errorf("Error() returned empty string, want a non-empty message")
	}
}
