// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// testThemeRegistry is a small in-memory registry: a Dropdown, a ColorPicker,
// and a Toggle on DivContainer (what MDL `container` resolves to).
func testThemeRegistry() *ThemeRegistry {
	return &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{
		"DivContainer": {
			{Name: "Background color", Type: "Dropdown", Options: []ThemeOption{{Name: "Brand Primary"}, {Name: "Default"}}},
			{Name: "Text alignment", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Center"}, {Name: "Right"}}},
			{Name: "Card style", Type: "Toggle"},
		},
	}}
}

// TestAstDesignPropToValue_Typed verifies the write path picks the BSON value
// type from the theme registry: a value that is one of the property's declared
// options is stored as "option" for every control type (Dropdown,
// ToggleButtonGroup, ColorPicker swatch); only a ColorPicker's off-list value is
// "custom". A ToggleButtonGroup selection must NOT become custom or Studio Pro
// fails CE6084 ("Expected … Toggle button group, but found Custom"). Typed design
// properties.
func TestAstDesignPropToValue_Typed(t *testing.T) {
	props := []ThemeProperty{
		{Name: "Background color", Type: "Dropdown", Options: []ThemeOption{{Name: "Brand Primary"}, {Name: "Default"}}},
		{Name: "Text alignment", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Center"}, {Name: "Right"}}},
		{Name: "Column gap", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Medium"}, {Name: "Large"}}},
		{Name: "Accent", Type: "ColorPicker", Options: []ThemeOption{{Name: "Brand Primary"}}},
	}
	cases := []struct {
		key, val, wantType string
	}{
		{"Background color", "Brand Primary", "option"},
		{"Text alignment", "Center", "option"}, // ToggleButtonGroup option → option (not custom!)
		{"Column gap", "Medium", "option"},     // the CE6084 regression case
		{"Accent", "Brand Primary", "option"},  // ColorPicker predefined swatch → option
		{"Accent", "#ff0000", "custom"},        // ColorPicker free-form color → custom
		{"Unknown Key", "x", "option"},         // not in registry → default option
	}
	for _, c := range cases {
		dp, ok := astDesignPropToValue(ast.DesignPropertyEntryV3{Key: c.key, Value: c.val}, props)
		if !ok {
			t.Fatalf("%s: expected ok", c.key)
		}
		if dp.ValueType != c.wantType {
			t.Errorf("%s=%q: ValueType=%q, want %q", c.key, c.val, dp.ValueType, c.wantType)
		}
	}
	// on/off still map to toggle/skip regardless of metadata.
	if dp, ok := astDesignPropToValue(ast.DesignPropertyEntryV3{Key: "Card style", Value: "on"}, props); !ok || dp.ValueType != "toggle" {
		t.Errorf("on → toggle, got %+v ok=%v", dp, ok)
	}
	if _, ok := astDesignPropToValue(ast.DesignPropertyEntryV3{Key: "Card style", Value: "off"}, props); ok {
		t.Error("off should be skipped")
	}
	// Without metadata the flat default stays option (backward compatible).
	if dp, _ := astDesignPropToValue(ast.DesignPropertyEntryV3{Key: "Text alignment", Value: "Center"}, nil); dp.ValueType != "option" {
		t.Errorf("no metadata → option, got %q", dp.ValueType)
	}
}

func designPropViolations(t *testing.T, src string, reg *ThemeRegistry) map[string]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	out := map[string]string{}
	for _, stmt := range prog.Statements {
		for _, v := range ValidateDesignPropertiesForStatement(stmt, reg) {
			out[v.RuleID] = v.Message + " || " + v.Suggestion
		}
	}
	return out
}

// TestValidateDesignProperties_UnknownKeyAndBadValue covers MDL-WIDGET11 (unknown
// design-property key) and MDL-WIDGET12 (value not allowed, listing allowed values).
func TestValidateDesignProperties_UnknownKeyAndBadValue(t *testing.T) {
	reg := testThemeRegistry()

	// Unknown key.
	v := designPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Nonexistent': 'x']) {}
}`, reg)
	if _, ok := v["MDL-WIDGET11"]; !ok {
		t.Errorf("expected MDL-WIDGET11 for unknown key, got %v", v)
	}

	// Bad value on a known Dropdown → MDL-WIDGET12 listing allowed values.
	v = designPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Background color': 'Bogus']) {}
}`, reg)
	msg, ok := v["MDL-WIDGET12"]
	if !ok {
		t.Fatalf("expected MDL-WIDGET12 for bad value, got %v", v)
	}
	if !strings.Contains(msg, "Brand Primary") || !strings.Contains(msg, "Default") {
		t.Errorf("expected allowed values listed, got: %s", msg)
	}

	// Valid key + valid value + valid toggle → no violations.
	v = designPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Background color': 'Brand Primary', 'Card style': on]) {}
}`, reg)
	if len(v) != 0 {
		t.Errorf("expected no violations for valid design properties, got %v", v)
	}
}

// TestValidateDesignProperties_WrongCaseHint verifies the case-sensitivity hint.
func TestValidateDesignProperties_WrongCaseHint(t *testing.T) {
	reg := testThemeRegistry()
	v := designPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['card style': on]) {}
}`, reg)
	msg, ok := v["MDL-WIDGET11"]
	if !ok {
		t.Fatalf("expected MDL-WIDGET11 for wrong-case key, got %v", v)
	}
	if !strings.Contains(msg, "case-sensitive") || !strings.Contains(msg, "Card style") {
		t.Errorf("expected case-sensitivity hint naming 'Card style', got: %s", msg)
	}
}

// TestValidateDesignProperties_UnknownWidgetSkipped verifies a widget type with
// no registry metadata (e.g. a pluggable widget) is not flagged.
func TestValidateDesignProperties_UnknownWidgetSkipped(t *testing.T) {
	// Registry only knows DataGrid; the container has no entry → skip.
	prog, errs := visitor.Build(`create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Something': 'x']) {}
}`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	// Swap the widget's resolved key out of the registry by using an empty registry.
	empty := &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{"DataGrid": {{Name: "X", Type: "Toggle"}}}}
	var n int
	for _, stmt := range prog.Statements {
		n += len(ValidateDesignPropertiesForStatement(stmt, empty))
	}
	if n != 0 {
		t.Errorf("expected no violations when widget type has no registry metadata, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// The design-properties key of a keyword that writes a PLUGGABLE widget
// ---------------------------------------------------------------------------

// MDL-WIDGET11 resolved `datagrid` against Atlas Core's `DataGrid` — the
// DEPRECATED data grid — while MDL's `datagrid` has always written Data grid 2
// from the DataWidgets module. Their design properties are disjoint:
//
//	Atlas Core   DataGrid                                  Style, Hover style, Row size
//	DataWidgets  com.mendix.widget.web.datagrid.Datagrid   Borders, Compact, Hover, Striped
//
// So the tool warned that Compact / Hover / Striped were "not defined for this
// widget type" — they are exactly its properties — and suggested Style and Row
// size, which mxbuild refuses with CE6083 "not supported by your theme". Taking
// the advice turned 16 warnings into 17 build errors (ako/CapTrackV4 010).
//
// Three more were wrong in the quieter direction. `combobox`, `gallery` and
// `image` named keys that no web design-properties.json defines, so the registry
// lookup missed and validateWidgetDesignProps skipped those widgets entirely —
// silence that reads as approval.
//
// Each pairing below was measured by writing the widget and reading its type
// back out of the catalog, not inferred from the builder.
func TestResolveDesignPropsKey_PluggableKeywordsUseTheirWidgetID(t *testing.T) {
	for keyword, want := range map[string]string{
		"datagrid": "com.mendix.widget.web.datagrid.Datagrid",
		"gallery":  "com.mendix.widget.web.gallery.Gallery",
		"combobox": "com.mendix.widget.web.combobox.Combobox",
		"image":    "com.mendix.widget.web.image.Image",
	} {
		if got := resolveDesignPropsKey(keyword); got != want {
			t.Errorf("resolveDesignPropsKey(%q) = %q, want %q — design-properties.json "+
				"keys a pluggable widget by its id, and this keyword writes one",
				keyword, got, want)
		}
		// Case-insensitively too: the validator is handed whatever the author typed.
		if got := resolveDesignPropsKey(strings.ToUpper(keyword)); got != want {
			t.Errorf("resolveDesignPropsKey(%q) = %q, want %q", strings.ToUpper(keyword), got, want)
		}
	}
}

// CONTROL: the native widgets must keep their Atlas keys. A fix that routed
// every keyword through the widget registry would break these, and they are the
// majority — `container` alone carries most of the design properties an author
// ever writes.
func TestResolveDesignPropsKey_NativeKeywordsUnchanged(t *testing.T) {
	for keyword, want := range map[string]string{
		"container":         "DivContainer",
		"actionbutton":      "Button",
		"dataview":          "DataView",
		"listview":          "ListView",
		"layoutgrid":        "LayoutGrid",
		"referenceselector": "ReferenceSelector",
		"staticimage":       "StaticImageViewer",
	} {
		if got := resolveDesignPropsKey(keyword); got != want {
			t.Errorf("resolveDesignPropsKey(%q) = %q, want %q", keyword, got, want)
		}
	}
}

// CONTROL: an unrecognised type falls through unchanged, so a pluggable id
// written directly with PLUGGABLEWIDGET is already the key it needs to be.
func TestResolveDesignPropsKey_UnknownFallsThrough(t *testing.T) {
	const id = "com.example.widget.web.thing.Thing"
	if got := resolveDesignPropsKey(id); got != id {
		t.Errorf("resolveDesignPropsKey(%q) = %q, want it unchanged", id, got)
	}
}

// The two halves have to stay disjoint. A keyword named in both tables is a
// silent ambiguity: pluggableKeywordIDs wins, so the native entry becomes dead
// and the next person to edit it changes nothing.
func TestDesignPropsKeyTablesDoNotOverlap(t *testing.T) {
	for keyword := range pluggableKeywordIDs() {
		if native, ok := mdlKeywordToDesignPropsKey[keyword]; ok {
			t.Errorf("%q is in both tables (native %q and a pluggable id). "+
				"The native entry is dead — remove it.", keyword, native)
		}
	}
}

// The registry half must actually load. If NewWidgetRegistry ever fails here the
// map silently falls back to the dispatch table alone, and the three quiet cases
// go back to being skipped with no test failing.
func TestPluggableKeywordIDs_IncludesRegistryDefinitions(t *testing.T) {
	ids := pluggableKeywordIDs()
	if _, ok := ids["datagrid"]; !ok {
		t.Error("datagrid missing — the keyword dispatch table did not contribute")
	}
	if _, ok := ids["gallery"]; !ok {
		t.Error("gallery missing — the embedded widget definitions did not load, so " +
			"every registry-defined keyword silently keeps its old (wrong) key")
	}
}
