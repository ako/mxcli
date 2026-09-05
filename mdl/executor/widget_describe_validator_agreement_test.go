// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureProject is a real project with widgets/*.mpk committed. The .def.json
// CACHE under .mxcli/ is gitignored, so a test must not depend on it — which is
// the point here: the nine hand-crafted widgets never get a .def.json anyway,
// and they are exactly the ones this test is about.
func fixtureProject(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", "expr-checker", "minimal.mpr"))
	if err != nil {
		t.Skipf("cannot resolve fixture: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("fixture project not present: %v", err)
	}
	return p
}

// DESCRIBE WIDGET and the property validator must agree about which properties a
// widget has. They read different sources — DESCRIBE reads the project's
// installed .mpk, the validator reads the WidgetDefinition — and they disagreed.
//
// # The defect
//
// Nine widgets (combobox, gallery, image, barcodescanner, the four data-grid
// filters, dropdownsort) have hand-crafted definitions in sdk/widgets/definitions/
// and are deliberately never extracted per-project, so no .def.json is ever
// generated for them. Those hand-written definitions cover a fraction of the
// widget:
//
//	combobox        73 properties in the .mpk,  7 mapped + 4 known
//	gallery         44                         12
//	image           37                         14
//	barcodescanner  12                          1
//
// So `DESCRIBE WIDGET` emitted an example it labels "parses as written" — and it
// does parse — naming 33 properties that mxcli's OWN validator then rejected with
// MDL-WIDGET01. Since exec refuses before writing, barcodescanner could not be
// placed from MDL at all: include the five properties and exec refuses, omit them
// and mxbuild reports CE0463 "the definition of this widget has changed".
// Combobox was the sharpest case — its describe output marks `source` REQUIRED
// and its validator said the widget has no such property.
//
// Reported by an external test project against 41c55d09 + this PR, retested at
// bca5466e, and reproduced here at 64055caa: combobox 17, gallery 7,
// barcodescanner 5, image 4.
//
// # Why this is the right assertion
//
// Not "the example parses" — it already did. The generator and the validator are
// two readers of one widget, so the invariant is that a property one of them
// emits is not one the other calls nonexistent.
func TestDescribeWidgetPropertiesAreAcceptedByTheValidator(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}

	// The nine hand-crafted widgets are the population this is about; assert the
	// fixture actually has some of them installed, or the test proves nothing.
	targets := []string{"combobox", "gallery", "image", "barcodescanner"}
	var checked int
	var bad []string

	for _, name := range targets {
		desc, err := DescribeWidget(name, project)
		if err != nil {
			continue
		}
		if desc.Source != "project .mpk" {
			// Falling back to the embedded template means the .mpk is absent, and
			// then both sides read the same thin data and cannot disagree.
			continue
		}
		def, ok := registry.Get(name)
		if !ok || def == nil {
			continue
		}
		checked++

		allowed, _ := allowedWidgetProperties(def)
		known := knownUnmappedProperties(def, allowed)
		for _, p := range desc.Properties {
			if p.Key == "" || isSystemPropKey(p.Key) {
				continue
			}
			k := strings.ToLower(p.Key)
			if allowed[k] || known[k] {
				continue
			}
			bad = append(bad, name+"."+p.Key)
		}
	}

	if checked == 0 {
		t.Skip("none of the hand-crafted widgets are installed in the fixture with a .mpk")
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Errorf("%d properties DESCRIBE WIDGET emits are rejected by the validator "+
			"(MDL-WIDGET01 \"has no property\"); the two read different sources and must not "+
			"disagree:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}

// The control: enrichment must not make the validator accept ANYTHING. A property
// no widget declares is still an error, or MDL-WIDGET01 stops detecting typos —
// which is the rule's whole job.
func TestValidatorStillRejectsAPropertyTheMPKDoesNotDeclare(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}
	def, ok := registry.Get("combobox")
	if !ok || def == nil {
		t.Skip("combobox not in registry")
	}
	allowed, _ := allowedWidgetProperties(def)
	known := knownUnmappedProperties(def, allowed)

	for _, bogus := range []string{"notarealproperty", "sourceX", "optionsSourceTypo"} {
		k := strings.ToLower(bogus)
		if allowed[k] || known[k] {
			t.Errorf("%q was accepted; enrichment must add the widget's REAL properties, "+
				"not open the gate", bogus)
		}
	}
}
