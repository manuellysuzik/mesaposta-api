package postgres

import "testing"

// testHydrationTarget is a plain, package-private struct used only to unit
// test setUnexportedField in isolation, with no dependency on any real
// domain aggregate or database connection -- see hydrate.go's own doc
// comment for why this package needs setUnexportedField at all.
type testHydrationTarget struct {
	counter int
	label   string
}

func TestSetUnexportedField_SetsPrivateFieldsDirectly(t *testing.T) {
	target := testHydrationTarget{}

	setUnexportedField(&target, "counter", 42)
	setUnexportedField(&target, "label", "hydrated")

	if target.counter != 42 {
		t.Errorf("counter = %d, want 42", target.counter)
	}
	if target.label != "hydrated" {
		t.Errorf("label = %q, want %q", target.label, "hydrated")
	}
}

func TestSetUnexportedField_OverwritesExistingValue(t *testing.T) {
	target := testHydrationTarget{counter: 1, label: "before"}

	setUnexportedField(&target, "counter", 99)

	if target.counter != 99 {
		t.Errorf("counter = %d, want 99", target.counter)
	}
	if target.label != "before" {
		t.Errorf("label = %q, want unchanged %q (only counter was set)", target.label, "before")
	}
}
