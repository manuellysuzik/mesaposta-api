// Package operations is mesaposta-api's first Phase 1 domain module: it
// owns stock/inventory movements, purchase orders, and the financial
// transactions that flow from them. Like every module under
// internal/modules/**, it follows the layout, dependency rules (R1-R6), and
// testing split documented in internal/modules/README.md -- read that file
// before adding anything here.
//
// This file exists only to anchor the module's root package doc comment;
// no code lives directly under internal/modules/operations. See the
// domain/, application/, contracts/, and infrastructure/{http,postgres}/
// subpackages for the actual layout.
package operations
