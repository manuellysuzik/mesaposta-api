// Package bar is a fixture module package used to prove that R2 can detect
// internal/platform packages reaching into internal/modules.
package bar

// Value is an arbitrary exported symbol so package foo has something to
// reference.
const Value = 1
