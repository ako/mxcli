// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	mdlast "github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/pages" // registers the Forms$ types
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ownGroupThemeRegistry is the renamed-theme fixture plus the Atlas Core 4.1.3
// groups of the widgets whose MDL keyword had no theme-key mapping. Copied from
// themesource/atlas_core/web/design-properties.json, trimmed to what the tests
// touch. DivContainer is replaced wholesale: its "Background color" is a
// ColorPicker in Atlas, which is what makes a custom colour on `row`/`column`
// build-breaking.
func ownGroupThemeRegistry(t *testing.T) *ThemeRegistry {
	t.Helper()
	reg := renamedThemeRegistry(t)
	swatches := []ThemeOption{{Name: "Brand Primary"}, {Name: "Brand Secondary"}}
	reg.WidgetProperties["GroupBox"] = []ThemeProperty{
		{Name: "Style", Type: "ColorPicker", Options: swatches},
		{Name: "Callout style", Type: "Toggle"},
	}
	reg.WidgetProperties["TabContainer"] = []ThemeProperty{
		{Name: "Style", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Pills"}, {Name: "Lined"}}},
		{Name: "Tab position", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Center"}}},
		{Name: "Justify", Type: "Toggle"},
	}
	for _, k := range []string{"NavigationTree", "MenuBar", "SimpleMenuBar"} {
		reg.WidgetProperties[k] = []ThemeProperty{{Name: "Hide icons", Type: "Toggle"}}
	}
	reg.WidgetProperties["DivContainer"] = []ThemeProperty{
		{Name: "Background color", Type: "ColorPicker", Options: swatches},
		{Name: "Card style", Type: "Toggle"},
	}
	reg.WidgetProperties["Button"] = []ThemeProperty{
		{Name: "Size", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Large"}}},
	}
	return reg
}

// Each of these keywords writes a widget whose theme group Atlas declares, and
// none of them resolved to it: the keyword fell through as-is ("groupbox"), no
// design-properties.json defines that key, and validateWidgetDesignProps
// returned early. A typo in a group box's design property checked clean and was
// written — Studio Pro then reports CE6083 "not supported by your theme".
func TestValidateDesignProperties_OwnGroupWidgetsAreChecked(t *testing.T) {
	reg := ownGroupThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  groupbox gb (Caption: 'G', DesignProperties: ['Nonexistent': 'x'])
  tabcontainer tc (DesignProperties: ['Nonexistent': 'x']) { tabpage tp (Caption: 'T') }
  navigationtree nt (profile: 'Responsive', DesignProperties: ['Nonexistent': on])
  menubar mb (profile: 'Responsive', DesignProperties: ['Nonexistent': on])
  simplemenubar smb (profile: 'Responsive', DesignProperties: ['Nonexistent': on])
  row r (DesignProperties: ['Nonexistent': on]) { column rc }
  column c (DesignProperties: ['Nonexistent': on])
  button b (Caption: 'B', DesignProperties: ['Nonexistent': on])
  header h (DesignProperties: ['Nonexistent': on])
}`, reg)

	for _, name := range []string{"gb", "tc", "nt", "mb", "smb", "r", "c", "b", "h"} {
		var found bool
		for _, v := range vs {
			if v.RuleID == "MDL-WIDGET11" && strings.Contains(v.Message, `widget "`+name+`"`) &&
				strings.Contains(v.Message, "not defined") {
				found = true
			}
		}
		if !found {
			t.Errorf("widget %q: undefined design property not reported — the widget was not checked", name)
		}
	}
}

// CONTROL: what Atlas offers each of those widgets must not warn — its own
// group's properties and the "Widget" base.
func TestValidateDesignProperties_OwnGroupValidPropsPass(t *testing.T) {
	reg := ownGroupThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  groupbox gb (Caption: 'G', DesignProperties: ['Style': 'Brand Primary', 'Callout style': on, 'Align self': 'Left'])
  tabcontainer tc (DesignProperties: ['Style': 'Pills', 'Tab position': 'Center', 'Justify': on]) { tabpage tp (Caption: 'T') }
  navigationtree nt (profile: 'Responsive', DesignProperties: ['Hide icons': on])
  menubar mb (profile: 'Responsive', DesignProperties: ['Hide icons': on])
  simplemenubar smb (profile: 'Responsive', DesignProperties: ['Hide icons': on])
  row r (DesignProperties: ['Background color': 'Brand Primary', 'Card style': on]) { column rc }
  column c (DesignProperties: ['Card style': on, 'Spacing': ['margin-bottom': 'M']])
  button b (Caption: 'B', DesignProperties: ['Size': 'Small'])
}`, reg)
	for _, v := range vs {
		t.Errorf("unexpected %s: %s", v.RuleID, v.Message)
	}
}

// The write path resolves the same key to TYPE each value. Unmapped, a group
// box's ColorPicker "Style" and a row's/column's DivContainer "Background color"
// were unknown to the builder, so a free colour fell through to the "option"
// default and mxbuild refused the page (measured on a copy of PedApp, 11.13.0):
//
//	[CE6085] "Unknown option #ff0000 for design property Style." at Group box 'gbCustom'
//	[CE6085] "Unknown option #00ff00 for design property Background color." at Container 'r1'
func TestBuildWidget_OwnGroupColourIsCustom(t *testing.T) {
	reg := ownGroupThemeRegistry(t)
	prog, errs := visitor.Build(`create page M.P (layout: Atlas_Core.Atlas_Default) {
  groupbox gbCustom (Caption: 'G', DesignProperties: ['Style': '#ff0000'])
  groupbox gbSwatch (Caption: 'G', DesignProperties: ['Style': 'Brand Primary'])
  row r1 (DesignProperties: ['Background color': '#00ff00']) { column rc }
  column k1 (DesignProperties: ['Background color': '#0000ff'])
}`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	want := map[string]string{"gbCustom": "custom", "gbSwatch": "option", "r1": "custom", "k1": "custom"}
	for _, w := range prog.Statements[0].(*mdlast.CreatePageStmtV3).Widgets {
		pb := &pageBuilder{backend: &mock.MockBackend{}, widgetScope: map[string]model.ID{}, themeRegistry: reg}
		built, err := pb.buildWidgetV3(w)
		if err != nil {
			t.Fatalf("%s: %v", w.Name, err)
		}
		dps := built.(interface{ GetBaseWidget() *pages.BaseWidget }).GetBaseWidget().DesignProperties
		if len(dps) != 1 {
			t.Fatalf("%s: %d design properties written, want 1", w.Name, len(dps))
		}
		if got := dps[0].ValueType; got != want[w.Name] {
			t.Errorf("%s: %q written as %q, want %q", w.Name, dps[0].Key, got, want[w.Name])
		}
	}
}

// A row inside a layoutgrid, a column inside a row, and a dataview's footer are
// SLOTS, not widgets: buildLayoutGridRowV3 / buildLayoutGridColumnV3 and the
// dataview footer split never call applyWidgetAppearance, so their design
// properties are dropped on write. Resolving the keyword as a DivContainer there
// would validate against a group the value never reaches; staying silent reads
// as approval of a value that is thrown away.
func TestValidateDesignProperties_SlotDesignPropsAreReportedDropped(t *testing.T) {
	reg := ownGroupThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  layoutgrid lg {
    row nestedRow (DesignProperties: ['Card style': on]) {
      column nestedCol (DesignProperties: ['Card style': on]) {
        dynamictext t (Content: 'x')
      }
    }
  }
  row topRow { column topRowCol (DesignProperties: ['Card style': on]) }
  dataview dv {
    footer ft (DesignProperties: ['Card style': on]) { dynamictext f (Content: 'x') }
  }
}`, reg)
	for _, name := range []string{"nestedRow", "nestedCol", "topRowCol", "ft"} {
		var found bool
		for _, v := range vs {
			if v.RuleID == "MDL-WIDGET07" && strings.Contains(v.Message, `"`+name+`"`) &&
				strings.Contains(v.Message, "dropped") {
				found = true
			}
		}
		if !found {
			t.Errorf("slot %q: design properties dropped on write but not reported (%d violations)", name, len(vs))
		}
	}
	for _, v := range vs {
		if v.RuleID != "MDL-WIDGET07" {
			t.Errorf("a slot must not be validated as a widget, got %s: %s", v.RuleID, v.Message)
		}
	}
}

// CONTROL: the same keywords as a CHILD of a pluggable widget belong to the
// widget's own object lists (a data grid `column`), which the pluggable engine
// builds — not a DivContainer and not a layout-grid slot. They must stay
// unvalidated, as before.
func TestValidateDesignProperties_PluggableChildKeywordsNotResolved(t *testing.T) {
	reg := ownGroupThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database M.E) {
    column colA (Attribute: Name, DesignProperties: ['Nonexistent': on])
  }
}`, reg)
	for _, v := range vs {
		t.Errorf("pluggable child reported: %s: %s", v.RuleID, v.Message)
	}
}

// The keyword table is DERIVED from what the builder writes: keyword → stored
// $Type → theme key. This holds the first hop to the builder itself — every
// keyword is built and its $Type compared — so the table cannot drift from it
// the way `radiobuttons` → "RadioButtons" did (the builder writes
// Forms$RadioButtonGroup; mxbuild accepts a "RadioButtonGroup" design property on
// it and refuses a "RadioButtons" one with CE6083).
func TestKeywordStorageTypesMatchBuilder(t *testing.T) {
	src := `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container k_container
  customcontainer k_customcontainer
  header k_header
  footer k_footer
  controlbar k_controlbar
  template k_template
  filter k_filter
  row k_row { column rc }
  column k_column
  actionbutton k_actionbutton (Caption: 'x')
  linkbutton k_linkbutton (Caption: 'x')
  button k_button (Caption: 'x')
  textbox k_textbox (Attribute: Name)
  textarea k_textarea (Attribute: Name)
  datepicker k_datepicker (Attribute: D)
  checkbox k_checkbox (Attribute: B)
  radiobuttons k_radiobuttons (Attribute: B)
  dropdown k_dropdown (Attribute: E)
  dataview k_dataview
  listview k_listview
  layoutgrid k_layoutgrid
  dynamictext k_dynamictext (Content: 'a')
  label k_label (Content: 'a')
  title k_title (Content: 'a')
  staticimage k_staticimage
  dynamicimage k_dynamicimage
  navigationlist k_navigationlist
  snippetcall k_snippetcall (Snippet: M.S)
  tabcontainer k_tabcontainer { tabpage tp (Caption: 'a') }
  groupbox k_groupbox (Caption: 'a')
  scrollcontainer k_scrollcontainer { region center { placeholder Main } }
  navigationtree k_navigationtree (profile: 'Responsive')
  menubar k_menubar (profile: 'Responsive')
  simplemenubar k_simplemenubar (profile: 'Responsive')
  placeholder k_placeholder
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	seen := map[string]bool{}
	for _, w := range prog.Statements[0].(*mdlast.CreatePageStmtV3).Widgets {
		kw := strings.ToLower(w.Type)
		seen[kw] = true
		pb := &pageBuilder{
			backend: &mock.MockBackend{}, widgetScope: map[string]model.ID{},
			paramEntityNames: map[string]string{}, paramScope: map[string]model.ID{},
			entityContext: "M.E",
			execCache: &executorCache{createdSnippets: map[string]*createdSnippetInfo{
				"M.S": {ID: "s1", Name: "S", ModuleName: "M"}}},
		}
		built, err := pb.buildWidgetV3(w)
		if err != nil {
			t.Errorf("%s: %v", kw, err)
			continue
		}
		if got, want := built.GetTypeName(), mdlKeywordStorageType[kw]; got != want {
			t.Errorf("keyword %q builds %s, but mdlKeywordStorageType says %q", kw, got, want)
		}
	}
	for kw := range mdlKeywordStorageType {
		if !seen[kw] {
			t.Errorf("mdlKeywordStorageType has %q but this test does not build it", kw)
		}
	}
}

// Every keyword buildWidgetV3 dispatches on must be in mdlKeywordStorageType or
// be named here with the reason it writes no native widget of its own. Read
// from the switch's source so a new `case` cannot land without a decision.
func TestKeywordStorageTypesCoverBuilderDispatch(t *testing.T) {
	notANativeWidget := map[string]string{
		"legacydatagrid": "refused (not implemented)",
		"slot":           "refused outside a fragment body",
		"tabpage":        "refused outside a tabcontainer",
		"region":         "refused outside a scrollcontainer",
		"item":           "refused outside a navigationlist",
		"text":           "refused: writes Forms$Text, which Mendix does not have",
		"statictext":     "refused: writes Forms$Text, which Mendix does not have",
		"image":          "pluggable (com.mendix.widget.web.image.Image) — pluggableKeywordIDs",
	}
	for _, kw := range buildWidgetV3Cases(t) {
		_, mapped := mdlKeywordStorageType[kw]
		_, exempt := notANativeWidget[kw]
		if mapped == exempt {
			t.Errorf("buildWidgetV3 case %q: mapped=%v exempt=%v — exactly one must hold", kw, mapped, exempt)
		}
	}
}

func buildWidgetV3Cases(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "cmd_pages_builder_v3.go", nil, 0)
	if err != nil {
		t.Fatalf("parse builder source: %v", err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "buildWidgetV3" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					s, _ := strconv.Unquote(lit.Value)
					out = append(out, s)
				}
			}
			return true
		})
		return false
	})
	if len(out) < 20 {
		t.Fatalf("found only %d cases in buildWidgetV3 — the source walk is broken, not the table", len(out))
	}
	sort.Strings(out)
	return out
}

// Every theme key is a Mendix CLASS name: the qualified name, not the storage
// name. Measured with probe groups added to a copy of PedApp's theme (11.13.0):
// mxbuild accepts a "TabContainer" / "RadioButtonGroup" / "SnippetCallWidget"
// design property and refuses "TabControl" / "RadioButtons" / "SnippetCall" with
// CE6083 "not supported by your theme". The generated metamodel carries exactly
// that mapping (Forms$TabControl decodes into gen type TabContainer), so the key
// table is held to it — the one deliberate departure is an ANCESTOR class, which
// Studio Pro applies too (Atlas declares "Button"; a probe "ActionButton" group
// was accepted as well).
func TestDesignPropsKeysAreMetamodelClassNames(t *testing.T) {
	ancestor := map[string]string{"ActionButton": "Button"}
	// Keyed by the Forms$ spelling: the generated metamodel registers only that
	// prefix. The Pages$ twins in bsonTypeToDesignPropsKey are the same classes.
	for suffix, key := range storageTypeThemeKeys {
		storage := "Forms$" + suffix
		factory, ok := codec.DefaultRegistry.Lookup(storage)
		if !ok {
			t.Errorf("%s is not a type in the metamodel — nothing is ever stored as it", storage)
			continue
		}
		class := reflect.TypeOf(factory()).Elem().Name()
		want := class
		if a, ok := ancestor[class]; ok {
			want = a
		}
		if key != want {
			t.Errorf("%s (class %s) maps to theme key %q, want %q", storage, class, key, want)
		}
	}
}

// Both prefixes and each keyword's $Type resolve, so the derivation never falls
// through to the bare keyword for a native widget.
func TestResolveDesignPropsKey_NativeKeywords(t *testing.T) {
	want := map[string]string{
		"groupbox": "GroupBox", "tabcontainer": "TabContainer", "navigationtree": "NavigationTree",
		"menubar": "MenuBar", "simplemenubar": "SimpleMenuBar", "row": "DivContainer",
		"column": "DivContainer", "header": "DivContainer", "footer": "DivContainer",
		"button": "Button", "radiobuttons": "RadioButtonGroup", "snippetcall": "SnippetCallWidget",
		"label": "Label", "container": "DivContainer", "dynamicimage": "DynamicImageViewer",
	}
	for kw, key := range want {
		if got := resolveDesignPropsKey(strings.ToUpper(kw)); got != key {
			t.Errorf("resolveDesignPropsKey(%q) = %q, want %q", kw, got, key)
		}
	}
	for kw, storage := range mdlKeywordStorageType {
		if _, ok := bsonTypeToDesignPropsKey[storage]; !ok {
			t.Errorf("keyword %q writes %s, which has no theme key", kw, storage)
		}
	}
}
