// Package identity is the identity module's root: see domain/, application/,
// contracts/ and infrastructure/{http,postgres} for the actual code. This
// file exists only so `go doc` has somewhere to land for the module as a
// whole; see internal/modules/README.md for the layout every module
// (including this one) follows.
//
// identity is the module that owns mesaposta-api's first users table and
// the first real login/refresh/logout HTTP endpoints, connecting the
// already-tested-in-isolation internal/platform/auth/{token,refresh,password}
// building blocks to a real domain: see this module's own domain, ports and
// application packages for how "who is allowed to sign in, as which role,
// for which tenant" is modeled and enforced.
package identity
