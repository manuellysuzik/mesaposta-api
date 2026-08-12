// Package internal holds module b's private implementation. It must never
// be imported directly by other modules -- only b/contracts is allowed to
// be imported cross-module.
package internal

// Secret is an arbitrary exported symbol used only to prove the fixture
// import in package a is exercised.
const Secret = "b-internal"
