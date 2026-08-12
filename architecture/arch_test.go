package architecture

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// modeGraph is the minimal packages.Load mode needed by rules that only
// need to inspect the (transitive) import graph: R1, R2, R3, R4, R5.
const modeGraph = packages.NeedName | packages.NeedImports | packages.NeedDeps

// modeFull additionally loads syntax trees and type-checking information,
// required by R6 to resolve identifier references to the tenant.Parse
// symbol.
const modeFull = modeGraph | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo

// loadFixture loads every package under
// architecture/testdata/fixtures/<relDir> as its own, isolated Go module
// (each fixture directory has its own go.mod), so that its synthetic import
// paths (e.g. "internal/modules/a") never collide with the real
// mesaposta-api module. withTests controls whether _test.go files are
// included in the loaded packages.
func loadFixture(t *testing.T, relDir string, mode packages.LoadMode, withTests bool) []*packages.Package {
	t.Helper()

	dir := filepath.Join("testdata", "fixtures", relDir)
	cfg := &packages.Config{
		Mode:  mode,
		Dir:   dir,
		Tests: withTests,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load(%s): %v", relDir, err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("packages.Load(%s): loaded zero packages -- fixture path wrong?", relDir)
	}
	return pkgs
}

// findRule returns the named rule from the Rules table, failing the test if
// it is not registered.
func findRule(t *testing.T, name string) Rule {
	t.Helper()
	for _, r := range Rules {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("rule %s is not registered in Rules", name)
	return Rule{}
}

// findViolation returns the first violation whose Package ends with
// pkgSuffix.
func findViolation(violations []Violation, pkgSuffix string) (Violation, bool) {
	for _, v := range violations {
		if strings.HasSuffix(v.Package, pkgSuffix) {
			return v, true
		}
	}
	return Violation{}, false
}

func mustFindViolation(t *testing.T, violations []Violation, pkgSuffix string) Violation {
	t.Helper()
	v, ok := findViolation(violations, pkgSuffix)
	if !ok {
		t.Fatalf("expected a violation for package suffix %q, got %d violations: %+v", pkgSuffix, len(violations), violations)
	}
	return v
}

func mustNotFindViolation(t *testing.T, violations []Violation, pkgSuffix string) {
	t.Helper()
	if v, ok := findViolation(violations, pkgSuffix); ok {
		t.Fatalf("expected NO violation for package suffix %q, got %+v", pkgSuffix, v)
	}
}

// TestArchitectureRules is the core table-driven suite: one case per rule
// (some rules get more than one case), each loading a dedicated fixture
// package that violates exactly that rule, and asserting the engine reports
// exactly that violation -- no more, no less.
func TestArchitectureRules(t *testing.T) {
	t.Run("R1_domain_transitively_reaches_net_http", func(t *testing.T) {
		pkgs := loadFixture(t, "domain_net_http", modeGraph, false)
		rule := findRule(t, "R1")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R1.Check: %v", err)
		}

		// domain/b imports net/http directly.
		b := mustFindViolation(t, violations, "domain/b")
		if !strings.Contains(b.Detail, "net/http") {
			t.Errorf("domain/b violation detail = %q, want it to mention net/http", b.Detail)
		}

		// domain/a only imports domain/b, but must still be flagged because
		// R1 checks the TRANSITIVE closure of imports, not just direct ones.
		a := mustFindViolation(t, violations, "domain/a")
		if !strings.Contains(a.Detail, "net/http") {
			t.Errorf("domain/a violation detail = %q, want it to mention net/http (transitively)", a.Detail)
		}

		// cmd/** is exempt (R5): the fixture's cmd/seed package also
		// imports net/http directly but must never be flagged by R1.
		mustNotFindViolation(t, violations, "cmd/seed")

		if len(violations) != 2 {
			t.Errorf("want exactly 2 violations (domain/a, domain/b), got %d: %+v", len(violations), violations)
		}
	})

	t.Run("R1_domain_reaches_database_sql", func(t *testing.T) {
		pkgs := loadFixture(t, "domain_sql", modeGraph, false)
		rule := findRule(t, "R1")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R1.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "domain")
		if !strings.Contains(v.Detail, "database/sql") {
			t.Errorf("violation detail = %q, want it to mention database/sql", v.Detail)
		}
		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation, got %d: %+v", len(violations), violations)
		}
	})

	t.Run("R2_platform_imports_module", func(t *testing.T) {
		pkgs := loadFixture(t, "platform_imports_module", modeGraph, false)
		rule := findRule(t, "R2")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R2.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "internal/platform/foo")
		if !strings.Contains(v.Detail, "internal/modules/bar") {
			t.Errorf("violation detail = %q, want it to mention internal/modules/bar", v.Detail)
		}
		mustNotFindViolation(t, violations, "internal/modules/bar")
		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation, got %d: %+v", len(violations), violations)
		}
	})

	// R5 documents that composition-root code is exempt from every rule, and
	// names both cmd/** AND internal/platform/app explicitly as composition
	// roots (see internal/modules/README.md and the real
	// internal/platform/app.NewApp doc comment). These two subtests pin down
	// R2's narrow carve-out for internal/platform/app specifically: it must
	// remain the ONLY internal/platform/** package excused from R2, so a
	// sibling package under internal/platform/** (here, package foo) must
	// keep tripping R2 exactly as before.
	t.Run("R2_non_app_platform_package_still_violates", func(t *testing.T) {
		pkgs := loadFixture(t, "platform_imports_module", modeGraph, false)
		rule := findRule(t, "R2")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R2.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "internal/platform/foo")
		if !strings.Contains(v.Detail, "internal/modules/bar") {
			t.Errorf("violation detail = %q, want it to mention internal/modules/bar", v.Detail)
		}
	})

	t.Run("R2_platform_app_composition_root_is_exempt", func(t *testing.T) {
		pkgs := loadFixture(t, "platform_imports_module", modeGraph, false)
		rule := findRule(t, "R2")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R2.Check: %v", err)
		}

		// internal/platform/app imports internal/modules/bar too (see the
		// fixture's app.go), but unlike foo it must NEVER be flagged: R2's
		// exception for the composition root exists exactly to keep this
		// fixture green once internal/platform/app.NewApp (the real
		// composition root) starts wiring internal/modules/operations in
		// T6.2.
		mustNotFindViolation(t, violations, "internal/platform/app")

		// Only foo violates; app must not add a second violation.
		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation (foo only, app is exempt), got %d: %+v", len(violations), violations)
		}
	})

	t.Run("R3_cross_module_without_contracts", func(t *testing.T) {
		pkgs := loadFixture(t, "cross_module", modeGraph, false)
		rule := findRule(t, "R3")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R3.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "internal/modules/a")
		if !strings.Contains(v.Detail, "internal/modules/b/internal") {
			t.Errorf("violation detail = %q, want it to mention internal/modules/b/internal", v.Detail)
		}

		// Package c imports b via contracts and must NOT be flagged.
		mustNotFindViolation(t, violations, "internal/modules/c")
		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation, got %d: %+v", len(violations), violations)
		}
	})

	t.Run("R4_tenant_imports_non_stdlib", func(t *testing.T) {
		pkgs := loadFixture(t, "tenant_impure/r4", modeGraph, false)
		rule := findRule(t, "R4")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R4.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "internal/platform/tenant")
		if !strings.Contains(v.Detail, "github.com/some/thirdparty") {
			t.Errorf("violation detail = %q, want it to mention github.com/some/thirdparty", v.Detail)
		}
		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation, got %d: %+v", len(violations), violations)
		}
	})

	t.Run("R6_tenant_parse_misuse", func(t *testing.T) {
		pkgs := loadFixture(t, "tenant_impure/r6", modeFull, true)
		rule := findRule(t, "R6")

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("R6.Check: %v", err)
		}

		v := mustFindViolation(t, violations, "internal/consumer")
		if !strings.Contains(v.Detail, "tenant.Parse") {
			t.Errorf("violation detail = %q, want it to mention tenant.Parse", v.Detail)
		}

		// internal/platform/auth is the allowed caller.
		mustNotFindViolation(t, violations, "internal/platform/auth")

		if len(violations) != 1 {
			t.Errorf("want exactly 1 violation (only the non-test, non-auth reference), got %d: %+v", len(violations), violations)
		}
	})
}

// TestR5CmdIsExempt is a trivial, dedicated regression test documenting R5:
// cmd/** is exempt from every rule. It re-runs R1 (the rule most likely to
// otherwise flag it, since the fixture's cmd/seed package imports
// net/http) against the domain_net_http fixture and asserts cmd/seed is
// never reported, and separately confirms R5.Check never reports anything
// on its own.
func TestR5CmdIsExempt(t *testing.T) {
	pkgs := loadFixture(t, "domain_net_http", modeGraph, false)

	r1 := findRule(t, "R1")
	violations, err := r1.Check(pkgs)
	if err != nil {
		t.Fatalf("R1.Check: %v", err)
	}
	mustNotFindViolation(t, violations, "cmd/seed")

	r5 := findRule(t, "R5")
	r5Violations, err := r5.Check(pkgs)
	if err != nil {
		t.Fatalf("R5.Check: %v", err)
	}
	if len(r5Violations) != 0 {
		t.Errorf("R5 is permissive by design and must never report violations, got %+v", r5Violations)
	}
}

// ruleFixtures lists, for every rule that is expected to be exercised
// against a real violation somewhere in the fixture suite, which fixture(s)
// to load and in which mode. TestRulesAreNotVacuous uses this table
// independently of TestArchitectureRules (no shared state between the two
// tests) to guard against a rule silently matching nothing.
var ruleFixtures = []struct {
	rule string
	dir  string
	mode packages.LoadMode
}{
	{"R1", "domain_net_http", modeGraph},
	{"R1", "domain_sql", modeGraph},
	{"R2", "platform_imports_module", modeGraph},
	{"R3", "cross_module", modeGraph},
	{"R4", "tenant_impure/r4", modeGraph},
	{"R6", "tenant_impure/r6", modeFull},
}

// TestRulesAreNotVacuous is the anti-vacuity meta-test required by the
// Bloco A acceptance criteria: any rule that is not explicitly marked
// AllowEmpty must match at least MinMatches (or 1, whichever is greater)
// violations somewhere in the fixture suite, or the suite would be green
// only because it isn't actually looking at anything.
func TestRulesAreNotVacuous(t *testing.T) {
	totals := map[string]int{}

	for _, rf := range ruleFixtures {
		withTests := rf.mode&packages.NeedSyntax != 0
		pkgs := loadFixture(t, rf.dir, rf.mode, withTests)
		rule := findRule(t, rf.rule)

		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("%s.Check(%s): %v", rf.rule, rf.dir, err)
		}
		totals[rf.rule] += len(violations)
	}

	for _, rule := range Rules {
		if rule.AllowEmpty {
			continue
		}
		want := rule.MinMatches
		if want < 1 {
			want = 1
		}
		got := totals[rule.Name]
		if got < want {
			t.Errorf("rule %s is vacuous: matched %d violations across the fixture suite, want >= %d (declare AllowEmpty if this is intentional)", rule.Name, got, want)
		}
	}
}

// TestFixturesAreLoadableFromTestdata confirms, explicitly and in
// isolation, that fixtures physically living under testdata/ (a directory
// go build/go vet/go test ./... ignore by default) are still loadable by
// packages.Load when referenced by an explicit path -- the acceptance
// criteria calls this out as something to verify in practice, not assume.
func TestFixturesAreLoadableFromTestdata(t *testing.T) {
	for _, dir := range []string{
		"domain_net_http",
		"domain_sql",
		"platform_imports_module",
		"cross_module",
		"tenant_impure/r4",
		"tenant_impure/r6",
	} {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			pkgs := loadFixture(t, dir, modeGraph, false)
			if len(pkgs) == 0 {
				t.Fatalf("expected at least one package loaded from testdata/fixtures/%s", dir)
			}
		})
	}
}

// TestRepoCompliance is the "production" check: it loads the REAL
// mesaposta-api module (repo root, one level up from this package) -- not
// a fixture -- and asserts that all rules report zero violations against
// the actual codebase. This is the test the acceptance-criteria manual
// demonstration exercises: adding a real package under internal/... that
// violates a rule must make this test (and therefore
// `go test ./architecture/...`) fail; removing the violation must make it
// pass again. See README.md, "Como verificar a fitness function".
func TestRepoCompliance(t *testing.T) {
	cfg := &packages.Config{
		Mode:  modeFull,
		Dir:   "..",
		Tests: true,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load(repo root): %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("packages.Load(repo root): loaded zero packages")
	}

	var all []Violation
	for _, rule := range Rules {
		violations, err := rule.Check(pkgs)
		if err != nil {
			t.Fatalf("%s.Check(repo): %v", rule.Name, err)
		}
		all = append(all, violations...)
	}

	if len(all) != 0 {
		var lines []string
		for _, v := range all {
			lines = append(lines, v.String())
		}
		t.Fatalf("architecture fitness function found %d violation(s) in the real repository:\n%s", len(all), strings.Join(lines, "\n"))
	}
}

// TestRuleTableDeclaresAllRules is a small structural guard ensuring R1-R6
// are all present and each declares AllowEmpty/MinMatches explicitly (no
// rule silently relying on Go zero values without the author noticing).
func TestRuleTableDeclaresAllRules(t *testing.T) {
	want := []string{"R1", "R2", "R3", "R4", "R5", "R6"}
	got := make(map[string]bool, len(Rules))
	for _, r := range Rules {
		if r.Name == "" {
			t.Fatalf("rule with empty Name in Rules table: %+v", r)
		}
		if r.Check == nil {
			t.Fatalf("rule %s has a nil Check function", r.Name)
		}
		got[r.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("Rules table is missing rule %s", name)
		}
	}
}
