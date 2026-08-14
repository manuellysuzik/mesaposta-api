// Package architecture implements the mesaposta-api architecture fitness
// function: a table-driven set of rules (R1-R6) that inspect the module's
// package/import graph (via golang.org/x/tools/go/packages) and report any
// violation of the DDD tactical + Hexagonal boundaries the project commits
// to. See docs/adr/0002-arch-fitness-tooling.md for the rationale behind
// building this as a Go test suite instead of adopting an external tool.
package architecture

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Violation describes a single architecture rule violation found while
// inspecting a package.
type Violation struct {
	// Rule is the short rule identifier (e.g. "R1").
	Rule string
	// Package is the import path of the offending package.
	Package string
	// Detail is a human-readable explanation of what was found.
	Detail string
}

// String renders the violation for test failure messages and (eventually)
// CI output.
func (v Violation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.Rule, v.Package, v.Detail)
}

// Rule is a single architecture fitness rule.
type Rule struct {
	// Name is the short rule identifier (e.g. "R1").
	Name string
	// Description documents, in one sentence, what the rule enforces.
	Description string
	// AllowEmpty marks a rule whose target packages may not exist yet in
	// the current state of the codebase (e.g. internal/modules/** before
	// Phase 1 introduces it). Rules marked AllowEmpty are exempt from the
	// anti-vacuity meta-test (see architecture/arch_test.go,
	// TestRulesAreNotVacuous): it is fine for them to currently match
	// nothing in the real repository.
	AllowEmpty bool
	// MinMatches, when > 0, is the minimum number of violations this
	// rule's Check must report across the fixture suite for the rule to be
	// considered meaningfully exercised (used by the anti-vacuity
	// meta-test only -- it has no effect on production compliance runs,
	// where the goal is always zero violations).
	MinMatches int
	// Check inspects the given packages and returns any violations found.
	Check func(pkgs []*packages.Package) ([]Violation, error)
}

// Rules is the full, ordered set of architecture fitness rules enforced by
// this package.
var Rules = []Rule{
	{
		Name: "R1",
		Description: "packages under **/domain/** must not transitively reach net/http, database/sql, " +
			"github.com/jackc/**, github.com/go-chi/**, github.com/golang-jwt/**, github.com/pressly/**, " +
			"github.com/alexedwards/**, nor internal/platform/** outside the {errs, tenant, outbox, audit} allowlist",
		AllowEmpty: false,
		MinMatches: 1,
		Check:      checkR1,
	},
	{
		Name: "R2",
		Description: "internal/platform/** must not import internal/modules/**, except internal/platform/app " +
			"(the composition root, exempt per R5)",
		AllowEmpty: false,
		MinMatches: 1,
		Check:      checkR2,
	},
	{
		Name:        "R3",
		Description: "internal/modules/<a>/** must not import internal/modules/<b>/** except via <b>/contracts",
		AllowEmpty:  false,
		MinMatches:  1,
		Check:       checkR3,
	},
	{
		Name:        "R4",
		Description: "internal/platform/errs and internal/platform/tenant must only import the standard library",
		AllowEmpty:  false,
		MinMatches:  1,
		Check:       checkR4,
	},
	{
		Name:        "R5",
		Description: "cmd/** may import anything -- this rule documents the exception, it never reports violations",
		AllowEmpty:  true,
		Check:       checkR5,
	},
	{
		Name:        "R6",
		Description: "only internal/platform/auth/** and _test.go files may reference tenant.Parse",
		AllowEmpty:  false,
		MinMatches:  1,
		Check:       checkR6,
	},
}

// --- shared helpers ---------------------------------------------------

// containsSegment reports whether pkgPath has segment as one of its
// "/"-separated path components.
func containsSegment(pkgPath, segment string) bool {
	for _, part := range strings.Split(pkgPath, "/") {
		if part == segment {
			return true
		}
	}
	return false
}

// isUnderPath reports whether base appears as a proper path-segment
// sequence anywhere in pkgPath (e.g. isUnderPath("a/b/internal/platform/foo",
// "internal/platform") is true).
// isAppTestVariant reports whether pkgPath is one of the synthetic package
// identities Go's loader produces for internal/platform/app's own test
// files: the external test package ("internal/platform/app_test", from
// app_test.go's "package app_test" declaration) or the compiled test
// binary ("internal/platform/app.test"). See checkR2's doc comment for why
// these need their own check instead of falling under isUnderPath's
// directory-segment definition of "internal/platform/app".
func isAppTestVariant(pkgPath string) bool {
	return strings.HasSuffix(pkgPath, "internal/platform/app_test") ||
		strings.HasSuffix(pkgPath, "internal/platform/app.test")
}

func isUnderPath(pkgPath, base string) bool {
	if pkgPath == base {
		return true
	}
	if strings.HasSuffix(pkgPath, "/"+base) {
		return true
	}
	idx := strings.Index(pkgPath, base+"/")
	if idx == -1 {
		return false
	}
	return idx == 0 || pkgPath[idx-1] == '/'
}

// transitiveImports returns every package transitively reachable from pkg
// (excluding pkg itself), keyed by import path. Requires the packages to
// have been loaded with NeedImports|NeedDeps.
func transitiveImports(pkg *packages.Package) map[string]*packages.Package {
	result := map[string]*packages.Package{}
	visited := map[string]bool{}

	var walk func(p *packages.Package)
	walk = func(p *packages.Package) {
		for impPath, impPkg := range p.Imports {
			if visited[impPath] {
				continue
			}
			visited[impPath] = true
			result[impPath] = impPkg
			walk(impPkg)
		}
	}
	walk(pkg)

	return result
}

// sortedKeys returns the sorted string keys of a map[string]*packages.Package.
func sortedKeys(m map[string]*packages.Package) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- R1: domain isolation ----------------------------------------------

// forbiddenImportPrefixes lists import paths (or path prefixes ending in
// "/") that no package under **/domain/** may transitively reach.
var forbiddenImportPrefixes = []string{
	"net/http",
	"database/sql",
	"github.com/jackc/",
	"github.com/go-chi/",
	"github.com/golang-jwt/",
	"github.com/pressly/",
	"github.com/alexedwards/",
}

// domainPlatformAllowlist is the set of internal/platform/** subpackages a
// domain package is allowed to import.
//
// Every entry here is a PURE PORT package: types and interfaces only, with
// no pgx/chi/net-http/database-sql anywhere in its own build graph. That
// property is the entire admission criterion -- adding a package that
// transitively reaches infrastructure would let a domain package import it
// and silently smuggle the whole forbidden set past this rule. Note that
// "outbox" and "audit" name only the port packages themselves; their
// Postgres adapters deliberately live in the separate
// internal/platform/{outbox,audit}/postgres subpackages, which are NOT on
// this allowlist (platformSubpackage returns the first segment after
// "internal/platform/", so "internal/platform/audit/postgres" resolves to
// "audit" -- see the doc comment on internal/platform/audit/postgres for
// why the split exists, and note that a domain package importing that
// subpackage would still be caught by forbiddenImportPrefixes, since it
// transitively reaches github.com/jackc/**).
var domainPlatformAllowlist = map[string]bool{
	"errs":   true,
	"tenant": true,
	"outbox": true,
	"audit":  true,
}

// matchForbiddenImport reports whether importPath matches one of
// forbiddenImportPrefixes, returning the matched prefix.
func matchForbiddenImport(importPath string) (string, bool) {
	for _, prefix := range forbiddenImportPrefixes {
		if strings.HasSuffix(prefix, "/") {
			if strings.HasPrefix(importPath, prefix) {
				return prefix, true
			}
			continue
		}
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return prefix, true
		}
	}
	return "", false
}

// platformSubpackage returns the first path segment after
// ".../internal/platform/", if importPath is under internal/platform/.
func platformSubpackage(importPath string) (string, bool) {
	const marker = "internal/platform/"
	idx := strings.Index(importPath, marker)
	if idx == -1 {
		return "", false
	}
	if idx != 0 && importPath[idx-1] != '/' {
		return "", false
	}
	rest := importPath[idx+len(marker):]
	if rest == "" {
		return "", false
	}
	sub := strings.SplitN(rest, "/", 2)[0]
	if sub == "" {
		return "", false
	}
	return sub, true
}

// disallowedDomainPlatformImport reports whether importPath is an
// internal/platform/<sub> import not present in domainPlatformAllowlist.
func disallowedDomainPlatformImport(importPath string) (string, bool) {
	sub, ok := platformSubpackage(importPath)
	if !ok {
		return "", false
	}
	if domainPlatformAllowlist[sub] {
		return "", false
	}
	return sub, true
}

// checkR1 flags every package under **/domain/** that transitively reaches
// a forbidden import (see forbiddenImportPrefixes) or an internal/platform
// subpackage outside the allowlist. cmd/** and non-domain packages are
// never inspected (R5).
func checkR1(pkgs []*packages.Package) ([]Violation, error) {
	var violations []Violation

	for _, pkg := range pkgs {
		if !containsSegment(pkg.PkgPath, "domain") {
			continue
		}

		reached := transitiveImports(pkg)
		var found []string

		for _, impPath := range sortedKeys(reached) {
			if prefix, ok := matchForbiddenImport(impPath); ok {
				found = append(found, fmt.Sprintf("%s (via %s)", prefix, impPath))
				continue
			}
			if sub, ok := disallowedDomainPlatformImport(impPath); ok {
				found = append(found, fmt.Sprintf("internal/platform/%s (not in allowlist)", sub))
			}
		}

		if len(found) == 0 {
			continue
		}

		violations = append(violations, Violation{
			Rule:    "R1",
			Package: pkg.PkgPath,
			Detail:  fmt.Sprintf("reaches forbidden dependencies: %s", strings.Join(found, ", ")),
		})
	}

	return violations, nil
}

// --- R2: platform must not import modules -------------------------------

// checkR2 flags every internal/platform/** package that transitively
// imports internal/modules/**, EXCEPT internal/platform/app: the HTTP
// composition root. Per R5 (internal/modules/README.md and the doc comment
// on internal/platform/app.NewApp), internal/platform/app plays the same
// "wire everything together" role cmd/api does, and modules are expected to
// be plugged into it directly (see T6.2). This is the ONLY carve-out to
// R2 -- every other internal/platform/** package must stay clean.
//
// The exemption also covers internal/platform/app's own test packages
// (isAppTestVariant): Go's package loader (with Tests: true) reports the
// external test package for app as "internal/platform/app_test" and the
// compiled test binary as "internal/platform/app.test" -- neither PkgPath
// is "under" internal/platform/app by isUnderPath's directory-segment
// definition, so without this, app_test.go legitimately importing
// operations/application to build fixtures for the composition root's own
// tests would incorrectly trip R2.
func checkR2(pkgs []*packages.Package) ([]Violation, error) {
	var violations []Violation

	for _, pkg := range pkgs {
		if !isUnderPath(pkg.PkgPath, "internal/platform") {
			continue
		}
		if isUnderPath(pkg.PkgPath, "internal/platform/app") || isAppTestVariant(pkg.PkgPath) {
			continue
		}

		reached := transitiveImports(pkg)
		for _, impPath := range sortedKeys(reached) {
			if isUnderPath(impPath, "internal/modules") {
				violations = append(violations, Violation{
					Rule:    "R2",
					Package: pkg.PkgPath,
					Detail:  fmt.Sprintf("imports %s", impPath),
				})
				break
			}
		}
	}

	return violations, nil
}

// --- R3: modules must not import each other's internals -----------------

// moduleNameOf returns the module name segment right after
// ".../internal/modules/", if pkgPath is under internal/modules/.
func moduleNameOf(pkgPath string) (string, bool) {
	const marker = "internal/modules/"
	idx := strings.Index(pkgPath, marker)
	if idx == -1 {
		return "", false
	}
	if idx != 0 && pkgPath[idx-1] != '/' {
		return "", false
	}
	rest := pkgPath[idx+len(marker):]
	if rest == "" {
		return "", false
	}
	name := strings.SplitN(rest, "/", 2)[0]
	if name == "" {
		return "", false
	}
	return name, true
}

// isContractsImport reports whether pkgPath is (or is under)
// internal/modules/<module>/contracts.
func isContractsImport(pkgPath, module string) bool {
	prefix := "internal/modules/" + module + "/contracts"
	idx := strings.Index(pkgPath, prefix)
	if idx == -1 {
		return false
	}
	if idx != 0 && pkgPath[idx-1] != '/' {
		return false
	}
	end := idx + len(prefix)
	return end == len(pkgPath) || pkgPath[end] == '/'
}

// checkR3 flags every internal/modules/<a>/** package that transitively
// imports internal/modules/<b>/** (for a different module b) through any
// path other than <b>/contracts.
func checkR3(pkgs []*packages.Package) ([]Violation, error) {
	var violations []Violation

	for _, pkg := range pkgs {
		moduleA, ok := moduleNameOf(pkg.PkgPath)
		if !ok {
			continue
		}

		reached := transitiveImports(pkg)
		for _, impPath := range sortedKeys(reached) {
			moduleB, ok := moduleNameOf(impPath)
			if !ok || moduleB == moduleA {
				continue
			}
			if isContractsImport(impPath, moduleB) {
				continue
			}
			violations = append(violations, Violation{
				Rule:    "R3",
				Package: pkg.PkgPath,
				Detail:  fmt.Sprintf("imports %s (module %q) without going through %s/contracts", impPath, moduleB, moduleB),
			})
			break
		}
	}

	return violations, nil
}

// --- R4: platform/errs and platform/tenant must stay stdlib-only --------

// stdlibRestrictedPlatformPackages lists the internal/platform/**
// subpackages that may only import the standard library.
var stdlibRestrictedPlatformPackages = map[string]bool{
	"errs":   true,
	"tenant": true,
}

// isStdlib reports whether importPath looks like a standard-library import
// path, using the same heuristic goimports/gofmt use: stdlib import paths
// never have a "." in their first path segment (no domain-like prefix).
func isStdlib(importPath string) bool {
	first := importPath
	if idx := strings.Index(importPath, "/"); idx != -1 {
		first = importPath[:idx]
	}
	return !strings.Contains(first, ".")
}

// r4AllowedCrossImports documents the single carved-out exception to "errs
// and tenant may only import the standard library": internal/platform/tenant
// is allowed to import internal/platform/errs, so it can classify its
// validation/context errors using T03's error taxonomy (errs.Invalid,
// errs.Unauthenticated, ...) instead of duplicating that type. errs itself
// stays 100% stdlib (it never imports tenant), so the *combined* dependency
// surface of {errs, tenant} remains zero third-party imports -- which is
// R4's actual intent (see Bloco B task notes: "errs pode ser importado por
// tenant, mas ambos = zero import externo"). The isStdlib heuristic above
// cannot by itself tell "our own module" apart from a genuine third-party
// dependency (both have a "." in their first path segment), so this
// allowlist exists to encode that one specific, intentional exception
// without weakening R4 for anything else: any other non-stdlib import from
// either package (including a hypothetical tenant -> internal/platform/foo,
// or errs importing anything at all) is still flagged.
var r4AllowedCrossImports = map[string][]string{
	"tenant": {"internal/platform/errs"},
}

// isAllowedR4CrossImport reports whether importPath is (or is under) one of
// the documented exceptions in r4AllowedCrossImports for the
// stdlib-restricted platform subpackage named sub. Matching is done via
// isUnderPath (proper "/"-separated path-segment matching, not a raw
// prefix/suffix string check) because importPath is the full module-
// qualified path (e.g. "github.com/manuellysuzik/mesaposta-api/internal/
// platform/errs"), not the bare "internal/platform/errs" suffix.
func isAllowedR4CrossImport(sub, importPath string) bool {
	for _, allowed := range r4AllowedCrossImports[sub] {
		if isUnderPath(importPath, allowed) {
			return true
		}
	}
	return false
}

// checkR4 flags every internal/platform/errs or internal/platform/tenant
// package that directly imports a non-standard-library package, except for
// the single documented exception in r4AllowedCrossImports.
func checkR4(pkgs []*packages.Package) ([]Violation, error) {
	var violations []Violation

	for _, pkg := range pkgs {
		sub, ok := platformSubpackage(pkg.PkgPath)
		if !ok || !stdlibRestrictedPlatformPackages[sub] {
			continue
		}

		var nonStdlib []string
		for impPath := range pkg.Imports {
			if isStdlib(impPath) || isAllowedR4CrossImport(sub, impPath) {
				continue
			}
			nonStdlib = append(nonStdlib, impPath)
		}
		if len(nonStdlib) == 0 {
			continue
		}
		sort.Strings(nonStdlib)

		violations = append(violations, Violation{
			Rule:    "R4",
			Package: pkg.PkgPath,
			Detail:  fmt.Sprintf("imports non-stdlib package(s): %s", strings.Join(nonStdlib, ", ")),
		})
	}

	return violations, nil
}

// --- R5: cmd/** is exempt from everything --------------------------------

// checkR5 is intentionally a no-op: cmd/** (and everything else) may import
// anything. This rule exists purely to document that exemption in the
// Rules table; see TestR5CmdIsExempt for the regression test that proves
// other rules genuinely do not flag cmd/** packages.
func checkR5(_ []*packages.Package) ([]Violation, error) {
	return nil, nil
}

// --- R6: tenant.Parse callers -------------------------------------------

// checkR6 flags every reference to a func named Parse declared in an
// internal/platform/tenant package, found outside internal/platform/auth/**
// and outside _test.go files. Requires packages loaded with
// NeedSyntax|NeedTypes|NeedTypesInfo.
func checkR6(pkgs []*packages.Package) ([]Violation, error) {
	var violations []Violation
	seen := map[string]bool{} // dedupe across test-variant packages sharing the same source position

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for ident, obj := range pkg.TypesInfo.Uses {
			fn, ok := obj.(*types.Func)
			if !ok || fn.Name() != "Parse" {
				continue
			}
			if fn.Pkg() == nil || !isUnderPath(fn.Pkg().Path(), "internal/platform/tenant") {
				continue
			}

			pos := pkg.Fset.Position(ident.Pos())
			if strings.HasSuffix(pos.Filename, "_test.go") {
				continue
			}
			if isUnderPath(pkg.PkgPath, "internal/platform/auth") {
				continue
			}

			key := fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)
			if seen[key] {
				continue
			}
			seen[key] = true

			violations = append(violations, Violation{
				Rule:    "R6",
				Package: pkg.PkgPath,
				Detail:  fmt.Sprintf("references tenant.Parse at %s:%d:%d", pos.Filename, pos.Line, pos.Column),
			})
		}
	}

	return violations, nil
}
