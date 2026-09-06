// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mxcli-ledger FINDINGS §142, which took the CE0463 on a copied Atlas layout
// apart to one field. A full field-level diff of Atlas' own brand image against
// a describe → rename → exec copy of it, GUIDs masked, differs in **one line of
// 1480**:
//
//	=== ONLY IN ATLAS ===  Object/Properties/21/Value/PrimitiveValue = '250'
//	=== ONLY IN MINE ===   Object/Properties/21/Value/PrimitiveValue = '0'
//
// Resolved through its TypePointer that property is `maxHeight`, whose declared
// default in Image 1.6.0 is 250. mxcli writes 0. Setting it clears the build.
//
// This is the same class as §69 — a hidden property must hold its DECLARED
// DEFAULT, or the package rejects the widget — and the fix for §69 did not cover
// it, which is the interesting part. `hiddenUnnamedProperties` names maxHeight
// correctly: it walks the widget's VISIBILITY RULES, and maxHeight has two
// ("hidden when heightUnit ≠ auto", "hidden when maxHeightUnit = none").
//
// The reset was applied in a loop over the definition's PROPERTY MAPPINGS. A
// mapping is what gives a property an MDL keyword, and `width`/`height` have one
// (`Width:`, `Height:`) while `maxHeight` has none — MDL cannot spell it. So the
// property was never visited and the widget TEMPLATE's captured value stood.
//
// The set of properties that must be default-valued is decided by the widget's
// editorConfig, and the set MDL can name is decided by mxcli. Making one a subset
// of the other was the mistake: every hidden property with a declared default is
// written, whether or not MDL has a word for it.
//
// ---------------------------------------------------------------------------
//
// THAT FIX SHIPPED AND CHANGED NOTHING, for a reason worth stating plainly: it
// was verified against an input that does not exist. The helper below used to
// declare `maxHeightUnit`'s default as "none"; Image 1.6.0 declares "pixels" —
// so the test asked the rule a question the real package never asks.
// Re-measured against the .mpk in a live 11.14.0 project:
//
//	maxheight = "250"   maxheightunit = "pixels"
//
// With "pixels" the condition `maxHeightUnit = none` is FALSE, maxHeight reads
// as visible, no reset is written, and the template's 0 survives — which is
// exactly what the reporter kept seeing on a binary that contained the fix.
//
// The defect is which value the condition is evaluated against. `maxHeightUnit`
// is unmapped, so it is written with whatever the TEMPLATE captured — "none" —
// and never with its declared "pixels". Asking "is maxHeight hidden?" of the
// declared default asks about a document that will not be written; it has to be
// asked of the configuration that WILL be, which is what `stored` carries.
//
// Ground truth for the answer, measured across the 69 Image widgets in that
// project: all 65 carrying a `maxHeight` store **250**, at every combination of
// heightUnit and maxHeightUnit. The only outlier was the one mxcli authored.
// Proven both ways on the real package — patching that stored 0 to 250 takes the
// project from 1 error to 0, and restoring it takes it back to 1.

// imageDefWithMaxHeight is the Image widget reduced to the two properties this is
// about — one hidden property WITH an MDL mapping, one WITHOUT.
func imageDefWithMaxHeight() *WidgetDefinition {
	def := imageDef()
	def.PropertyVisibility = append(def.PropertyVisibility,
		// maxHeight's real rules on Image 1.5.0/1.6.0, both of them.
		types.WidgetVisibilityRule{PropertyKey: "maxHeight", HiddenWhen: &types.WidgetVisibilityCondition{
			PropertyKey: "maxHeightUnit", Operator: "eq", Value: "none"}},
	)
	return def
}

// The DECLARED defaults, as Image 1.6.0 states them — `maxHeightUnit` is
// "pixels". Reading "none" here is what let the first fix pass a test it could
// not pass in a project.
func imageDefaultsWithMaxHeight() map[string]string {
	d := imageDefaults()
	d["maxheight"] = "250"
	d["maxheightunit"] = "pixels"
	return d
}

// The TEMPLATE's captured configuration, as mxcli's embedded Image template
// holds it. `maxHeightUnit` is "none", which is NOT its declared default — a
// template captures whatever the widget it was extracted from happened to be set
// to, and that is precisely why an unmapped property cannot be reasoned about
// from the declared defaults.
func imageTemplateValues() map[string]string {
	return map[string]string{
		"heightUnit": "auto", "widthUnit": "auto",
		"maxHeightUnit": "none", "minHeightUnit": "none",
		"maxHeight": "0", "minHeight": "0",
	}
}

// The reset list must include a hidden property that has no MDL mapping. Before
// this, the caller iterated mappings, so `maxHeight` could be named here and
// still never written.
func TestHiddenResets_IncludeAPropertyMDLCannotName(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"},
		imageDefaultsWithMaxHeight(), imageTemplateValues())

	resets := unmappedHiddenResets(def, def.PropertyVisibility, hidden)
	got, ok := resets["maxHeight"]
	if !ok {
		t.Fatalf("maxHeight is hidden and has a declared default, but is not in the resets: %v", resets)
	}
	if got != "250" {
		t.Errorf("maxHeight reset = %q, want %q — the template's 0 is the CE0463", got, "250")
	}
}

// THE CONTROL that makes the test above mean anything, and the one the first
// attempt at this fix did not have. Drop the template's values and the rule is
// evaluated against the DECLARED default "pixels" instead — the condition is
// false, maxHeight reads as visible, and nothing is written. That is the exact
// state of the shipped binary the reporter measured: the fix present, the value
// still 0.
func TestHiddenResets_DeclaredDefaultAloneCannotAnswerTheCondition(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"},
		imageDefaultsWithMaxHeight(), nil)

	if v, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)["maxHeight"]; ok {
		t.Errorf("maxHeight was reset to %q from the declared defaults alone — if this "+
			"passes, the condition no longer depends on the template's value and the "+
			"test above proves nothing", v)
	}
}

// CONTROL 1: a hidden property that DOES have a mapping must not appear here.
// The mapping loop already writes it, and writing it twice would be a second
// value for one property.
func TestHiddenResets_SkipWhatTheMappingLoopAlreadyWrites(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"},
		imageDefaultsWithMaxHeight(), imageTemplateValues())

	for _, mapped := range []string{"width", "height"} {
		if _, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)[mapped]; ok {
			t.Errorf("%s has a property mapping and is written by that loop; "+
				"it must not be written a second time here", mapped)
		}
	}
}

// CONTROL 2: no declared default, no reset. mxcli does not invent a value — the
// rule is "hidden means default-valued", and with no default there is nothing to
// say. This is what keeps #956's File Uploader datasource pruning unchanged.
func TestHiddenResets_NoDeclaredDefaultWritesNothing(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	// The template still says maxHeightUnit is "none", so the property IS
	// hidden; what is missing is anything to reset it TO.
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"},
		imageDefaults(), imageTemplateValues())

	if v, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)["maxHeight"]; ok {
		t.Errorf("maxHeight was reset to %q with no declared default to reset it to", v)
	}
}

// CONTROL 3: a VISIBLE unmapped property is not touched. The reset is about
// hidden properties; applying it to a visible one would overwrite the template's
// value where the template is right.
//
// It also pins the precedence: the script's own `MaxHeightUnit: pixels` outranks
// the template's captured "none", or naming a property would stop meaning
// anything.
func TestHiddenResets_VisibleUnmappedPropertyIsLeftAlone(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	w := &ast.WidgetV3{Name: "img", Properties: map[string]any{"MaxHeightUnit": "pixels"}}
	hidden := e.hiddenUnnamedProperties(def, w,
		imageDefaultsWithMaxHeight(), imageTemplateValues())

	if _, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)["maxHeight"]; ok {
		t.Error("maxHeight is visible under this configuration and must keep its value")
	}
}
