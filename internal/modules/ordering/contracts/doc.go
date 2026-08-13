// Package contracts is the ONLY package of internal/modules/ordering other
// modules may ever import (architecture rule R3, see
// internal/modules/README.md). It is currently EMPTY: no other module in
// this codebase needs to integrate with ordering yet -- ordering is a
// consumer of catalog/contracts (for product pricing/menu listing), never
// the other way around, and it publishes no domain events another module
// subscribes to in this phase. Per internal/modules/README.md's own
// checklist ("Every module MUST have domain/, application/, contracts/
// even if contracts/ starts out nearly empty"), this package exists from
// the module's very first commit so the seam is already there, ready for
// whatever a future module genuinely needs from ordering, without ever
// tempting that future module to reach into ordering/domain,
// ordering/application, or ordering/infrastructure directly.
package contracts
