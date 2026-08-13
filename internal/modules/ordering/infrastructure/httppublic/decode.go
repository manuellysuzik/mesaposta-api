package httppublic

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const maxRequestBodyBytes = 1 << 20 // 1 MB

// decodeJSON unmarshals JSON from the request body into v, rejecting
// unknown fields and oversized bodies. Any failure results in an
// errs.Invalid error. Duplicated (not imported) from ../http's own
// decode.go: each package's _test.go files -- and, per this package's own
// doc comment, its entire DTO surface -- are deliberately independent.
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
