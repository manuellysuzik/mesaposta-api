// Package contracts is the ONLY package of internal/modules/community
// other modules may ever import (architecture rule R3, see
// internal/modules/README.md). It is deliberately small and stable:
// domain-event name constants and their payload DTOs (events.go) today,
// plus, as later phases add real cross-module use cases, whatever narrow
// types/interfaces another module genuinely needs to integrate with
// community -- and nothing else. This package must only ever import the
// standard library, so that depending on it never drags a module's
// internal dependencies (domain/, application/, infrastructure/) into
// another module's build graph.
package contracts
