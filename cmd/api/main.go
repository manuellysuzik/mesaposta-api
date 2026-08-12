// Command api is mesaposta-api's HTTP server entry point. All composition
// logic (config loading, database pool setup, router/middleware wiring,
// graceful shutdown) lives in run.go, kept testable as ordinary functions;
// main itself does nothing but wire the real process environment
// (os.Getenv, os.Stderr/os.Stdout, the real process signal handlers via
// context.Background) into run and translate its result into a process
// exit code.
package main

import (
	"context"
	"os"
)

func main() {
	os.Exit(run(context.Background(), os.Getenv, os.Stderr, os.Stdout))
}
