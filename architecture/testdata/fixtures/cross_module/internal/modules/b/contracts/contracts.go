// Package contracts is module b's public surface -- the only package other
// modules are allowed to import from module b.
package contracts

// Secret re-exports a safe, public value. Other modules may depend on this
// package freely.
const Secret = "b-public"
