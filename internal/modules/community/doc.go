// Package community is one of mesaposta-api's Phase 5 domain modules: it
// owns the tenant's internal community feed -- posts, comments on those
// posts, and likes on those posts. Like every module under
// internal/modules/**, it follows the layout, dependency rules (R1-R6), and
// testing split documented in internal/modules/README.md -- read that file
// before adding anything here.
//
// This file exists only to anchor the module's root package doc comment;
// no code lives directly under internal/modules/community. See the
// domain/, application/, contracts/, and infrastructure/{http,postgres}/
// subpackages for the actual layout.
package community
