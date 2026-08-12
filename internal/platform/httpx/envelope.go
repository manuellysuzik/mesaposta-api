package httpx

// Envelope is the standard JSON response body shape every mesaposta-api
// HTTP handler returns, success or failure:
//
//	{"data": ..., "error": {"code": ..., "message": ...} | null, "meta": {...}}
//
// Data and Error are mutually exclusive in practice (a success response has
// a nil Error, an error response has a nil Data), but both fields are
// always present in the encoded JSON -- absent-vs-null is not a signal
// clients should have to branch on. Meta is never nil: it always carries at
// least RequestID.
type Envelope struct {
	Data  any        `json:"data"`
	Error *ErrorBody `json:"error"`
	Meta  Meta       `json:"meta"`
}

// ErrorBody is the client-safe error payload nested under Envelope.Error.
// Code is a stable, machine-readable string a client can branch on; Message
// is a human-readable, client-safe description. Neither field is ever
// derived from an error's internal cause -- see errormap.go.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Meta carries response metadata every envelope includes. RequestID is
// always populated (falling back to "" only if genuinely unavailable, e.g.
// in a handwritten test that never went through the request-id middleware
// nor sent an X-Request-ID header). Pagination is nil for responses that
// don't page a collection.
type Meta struct {
	RequestID  string      `json:"request_id"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination describes a paginated collection response's metadata.
type Pagination struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}
