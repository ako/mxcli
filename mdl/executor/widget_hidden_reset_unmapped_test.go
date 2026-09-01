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

func imageDefaultsWithMaxHeight() map[string]string {
	d := imageDefaults()
	d["maxheight"] = "250"
	d["maxheightunit"] = "none"
	return d
}

// The reset list must include a hidden property that has no MDL mapping. Before
// this, the caller iterated mappings, so `maxHeight` could be named here and
// still never written.
func TestHiddenResets_IncludeAPropertyMDLCannotName(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"}, imageDefaultsWithMaxHeight())

	resets := unmappedHiddenResets(def, def.PropertyVisibility, hidden)
	got, ok := resets["maxHeight"]
	if !ok {
		t.Fatalf("maxHeight is hidden and has a declared default, but is not in the resets: %v", resets)
	}
	if got != "250" {
		t.Errorf("maxHeight reset = %q, want %q — the template's 0 is the CE0463", got, "250")
	}
}

// CONTROL 1: a hidden property that DOES have a mapping must not appear here.
// The mapping loop already writes it, and writing it twice would be a second
// value for one property.
func TestHiddenResets_SkipWhatTheMappingLoopAlreadyWrites(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"}, imageDefaultsWithMaxHeight())

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
	// Defaults for everything except maxHeight.
	hidden := e.hiddenUnnamedProperties(def, &ast.WidgetV3{Name: "img"}, imageDefaults())

	if v, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)["maxHeight"]; ok {
		t.Errorf("maxHeight was reset to %q with no declared default to reset it to", v)
	}
}

// CONTROL 3: a VISIBLE unmapped property is not touched. The reset is about
// hidden properties; applying it to a visible one would overwrite the template's
// value where the template is right.
func TestHiddenResets_VisibleUnmappedPropertyIsLeftAlone(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := imageDefWithMaxHeight()
	// maxHeightUnit = "pixels" makes maxHeight visible, so nothing is hidden by
	// that rule.
	w := &ast.WidgetV3{Name: "img", Properties: map[string]any{"MaxHeightUnit": "pixels"}}
	hidden := e.hiddenUnnamedProperties(def, w, imageDefaultsWithMaxHeight())

	if _, ok := unmappedHiddenResets(def, def.PropertyVisibility, hidden)["maxHeight"]; ok {
		t.Error("maxHeight is visible under this configuration and must keep its value")
	}
}
