package httpx

import "context"

// requestIDKey is an unexported type used exclusively as the context.Context
// key under which the current request's id is stored, following the same
// forge-proof pattern as internal/platform/tenant's context key.
type requestIDKey struct{}

var ctxRequestIDKey = requestIDKey{}

// WithRequestID returns a copy of ctx carrying id, retrievable via
// RequestIDFromContext. It does not mutate ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestIDKey, id)
}

// RequestIDFromContext extracts the request id previously attached to ctx
// via WithRequestID. It returns "" if ctx carries none.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxRequestIDKey).(string)
	return id
}
