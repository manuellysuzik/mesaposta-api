package httppublic_test

import "errors"

// errBoom is a sentinel used to simulate an unexpected upstream failure
// (e.g. catalog's MenuReader failing) in this package's handler tests.
var errBoom = errors.New("httppublic_test: boom")
