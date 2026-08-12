package httpx_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

func TestStatusFor_MapsEveryKnownKindToItsStatus(t *testing.T) {
	cases := []struct {
		kind errs.Kind
		want int
	}{
		{errs.Invalid, 400},
		{errs.Unauthenticated, 401},
		{errs.Forbidden, 403},
		{errs.NotFound, 404},
		{errs.Conflict, 409},
		{errs.TooManyRequests, 429},
		{errs.Internal, 500},
	}
	for _, tc := range cases {
		t.Run(tc.kind.String(), func(t *testing.T) {
			if got := httpx.StatusFor(tc.kind); got != tc.want {
				t.Fatalf("StatusFor(%v) = %d, want %d", tc.kind, got, tc.want)
			}
		})
	}
}

func TestStatusFor_UnknownKindFallsBackTo500(t *testing.T) {
	if got := httpx.StatusFor(errs.KindNone); got != 500 {
		t.Fatalf("StatusFor(KindNone) = %d, want 500", got)
	}
}

func TestErrorBodyFor_StructuredErrorUsesItsCodeAndPublicMessage(t *testing.T) {
	err := errs.New(errs.NotFound, "order_not_found", "order not found", errors.New("row not found: id=42"))

	status, body := httpx.ErrorBodyFor(err)

	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if body.Code != "order_not_found" || body.Message != "order not found" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestErrorBodyFor_RawErrorMapsToGenericInternal500(t *testing.T) {
	rawErrors := []error{
		errors.New("connection string: postgres://user:senha@host/db"),
		context.DeadlineExceeded,
	}

	for _, err := range rawErrors {
		status, body := httpx.ErrorBodyFor(err)

		if status != 500 {
			t.Fatalf("status = %d, want 500 for raw error %v", status, err)
		}
		if strings.Contains(body.Message, "senha") || strings.Contains(body.Code, "senha") {
			t.Fatalf("leaked sensitive content in body: %+v", body)
		}
	}
}

func TestErrorBodyFor_InternalKindStructuredErrorNeverLeaksCauseOrPublicMsg(t *testing.T) {
	err := errs.New(errs.Internal, "db_write_failed", "could not save order",
		errors.New("connection string: postgres://user:senha@host/db"))

	status, body := httpx.ErrorBodyFor(err)

	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	if strings.Contains(body.Message, "senha") || strings.Contains(body.Code, "senha") {
		t.Fatalf("leaked sensitive cause content: %+v", body)
	}
	if body.Message == "could not save order" {
		t.Fatalf("expected fixed generic internal message, got the caller-supplied Public() message")
	}
}

func TestNotFoundError_HasNotFoundKind(t *testing.T) {
	err := httpx.NotFoundError("order_not_found", "order not found", errors.New("cause"))
	if !errs.IsKind(err, errs.NotFound) {
		t.Fatalf("expected NotFound kind, got %v", errs.KindOf(err))
	}
}

func TestForbiddenError_HasForbiddenKind(t *testing.T) {
	err := httpx.ForbiddenError("insufficient_role", "you do not have permission", nil)
	if !errs.IsKind(err, errs.Forbidden) {
		t.Fatalf("expected Forbidden kind, got %v", errs.KindOf(err))
	}
}
