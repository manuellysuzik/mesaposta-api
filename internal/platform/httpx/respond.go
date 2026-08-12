package httpx

import (
	"encoding/json"
	"net/http"
)

// requestIDHeader is the canonical header name used to propagate a
// request's id to and from the client. net/http canonicalizes header names
// on both Set and Get, so callers may write or read it in any casing.
const requestIDHeader = "X-Request-ID"

// WriteJSON writes v as a JSON response body with the given status code and
// a "Content-Type: application/json" header. It is the lowest-level
// building block in this file; prefer RespondData/RespondError in handler
// code so every response goes through the standard Envelope shape.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// RespondData writes a successful response envelope carrying data at the
// given status code. pagination may be nil for non-paginated responses.
func RespondData(w http.ResponseWriter, r *http.Request, status int, data any, pagination *Pagination) {
	meta := metaFor(w, r, pagination)
	WriteJSON(w, status, Envelope{Data: data, Error: nil, Meta: meta})
}

// RespondError classifies err (see ErrorBodyFor) and writes the matching
// error response envelope, with Data left nil.
func RespondError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := ErrorBodyFor(err)
	meta := metaFor(w, r, nil)
	WriteJSON(w, status, Envelope{Data: nil, Error: &body, Meta: meta})
}

// metaFor resolves the current request's id -- preferring the value placed
// in the request context by the request-id middleware (see requestid.go,
// router.go), falling back to whatever the client sent in the X-Request-ID
// header for handlers/tests exercised outside the full middleware chain --
// sets it on the response header, and returns the Meta to embed in the
// envelope.
func metaFor(w http.ResponseWriter, r *http.Request, pagination *Pagination) Meta {
	id := RequestIDFromContext(r.Context())
	if id == "" {
		id = r.Header.Get(requestIDHeader)
	}
	if id != "" {
		w.Header().Set(requestIDHeader, id)
	}
	return Meta{RequestID: id, Pagination: pagination}
}
