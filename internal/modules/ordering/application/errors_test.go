package application_test

import "errors"

// errUnexpectedPersistCall is a sentinel a test injects into a fake
// repository's error slot for a write method it expects NEVER to be
// called, so that -- if the use case under test unexpectedly calls it
// anyway -- the test fails loudly instead of silently succeeding.
var errUnexpectedPersistCall = errors.New("application_test: unexpected persistence call")
