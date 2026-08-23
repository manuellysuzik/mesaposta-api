package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const maxRequestBodyBytes = 1 << 20 // 1 MB

// decodeJSON unmarshals JSON from the request body into v, rejecting unknown
// fields and oversized bodies. Any failure results in an errs.Invalid error.
// Mirrors internal/modules/catalog/infrastructure/http's own decodeJSON --
// duplicated rather than shared, per architecture rule R3 (no module's
// infrastructure/http may import another module's infrastructure/http).
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	lr := io.LimitReader(r.Body, maxRequestBodyBytes+1)
	dec := json.NewDecoder(lr)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		if err == io.EOF {
			return v, errs.New(errs.Invalid, "empty_request_body", "request body is required", err)
		}
		return v, errs.New(errs.Invalid, "invalid_request_body", "request body is not valid JSON", err)
	}
	return v, nil
}
