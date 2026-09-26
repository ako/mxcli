// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/model"
)

// storedListViewPage is a page holding one NATIVE List View — the widget whose
// design properties ako/mxcli#1135 was filed about.
func storedListViewPage() bson.D {
	lv := bson.D{
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Name", Value: "lvOrders"},
		{Key: "Appearance", Value: bson.D{
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: ""},
		}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), lv}}},
			}},
		}},
	}
}

func listViewMutator(t *testing.T, stored bson.D) backend.PageMutator {
	t.Helper()
	return pagemutator.New(stored, model.ID("u1"), &countingDeps{})
}

// atlasListViewTheme mirrors what Atlas declares for a List View on 11.12.2 —
// three of its own plus three inherited from the `Widget` group.
func atlasListViewTheme() *ThemeRegistry {
	return &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{
		"Widget": {
			{Name: "Align self", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Right"}}},
			{Name: "Hide on", Type: "ToggleButtonGroup", MultiSelect: true,
				Options: []ThemeOption{{Name: "Phone"}, {Name: "Tablet"}}},
		},
		"ListView": {
			{Name: "Row size", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Large"}}},
			{Name: "Hover style", Type: "Toggle", Class: "listview-hover"},
		},
		"com.mendix.widget.web.datagrid.Datagrid": {
			{Name: "Striped", Type: "Toggle", Class: "table-striped"},
		},
	}}
}

// ako/mxcli#515. The stored widget's type has to be resolvable before a design
// property can be told from a mistyped pluggable one.
func TestStoredWidgetDesignPropsKey(t *testing.T) {
	t.Run("native widget resolves from its $Type", func(t *testing.T) {
		m := listViewMutator(t, storedListViewPage())
		if got := storedWidgetDesignPropsKey(m, "lvOrders"); got != "ListView" {
			t.Errorf("key = %q, want ListView", got)
		}
	})

	t.Run("pluggable widget resolves to its widget id", func(t *testing.T) {
		m := listViewMutator(t, storedGridPage())
		if got := storedWidgetDesignPropsKey(m, "dgProducts"); got != "" {
			// storedGridPage()'s Type carries no WidgetId, so this is the
			// "cannot be established" case, which must NOT fall back to the
			// CustomWidgets$CustomWidget $Type — that is not a theme key and
			// would route every pluggable widget at whatever it happened to hit.
			t.Errorf("key = %q, want empty for a widget whose id is not stored", got)
		}
	})

	t.Run("unknown widget is empty, never a guess", func(t *testing.T) {
		m := listViewMutator(t, storedListViewPage())
		if got := storedWidgetDesignPropsKey(m, "noSuchWidget"); got != "" {
			t.Errorf("key = %q, want empty", got)
		}
	})
}

// The routing decision itself. nil means "leave SET where it was", which is what
// keeps a typo on the pluggable path and its own error.
func TestDesignPropertyForStoredWidget(t *testing.T) {
	m := listViewMutator(t, storedListViewPage())
	theme := atlasListViewTheme()

	if p := designPropertyForStoredWidget(theme, m, "lvOrders", "Row size"); p == nil {
		t.Error("a ListView design property was not recognised")
	}
	// Inherited from the `Widget` group — resolving only the type-specific list
	// is the mistake this area keeps producing.
	if p := designPropertyForStoredWidget(theme, m, "lvOrders", "Align self"); p == nil {
		t.Error("an INHERITED design property was not recognised")
	}
	// Declared for a different widget type: not this widget's, so not routed.
	if p := designPropertyForStoredWidget(theme, m, "lvOrders", "Striped"); p != nil {
		t.Error("a property of another widget type was routed to this one")
	}
	// A typo must stay on the pluggable path and get that path's error.
	if p := designPropertyForStoredWidget(theme, m, "lvOrders", "Rowsize"); p != nil {
		t.Error("a mistyped key was routed as a design property")
	}
	// No theme metadata at all: nothing can be claimed.
	if p := designPropertyForStoredWidget(nil, m, "lvOrders", "Row size"); p != nil {
		t.Error("claimed a design property with no theme to judge against")
	}
}

// The value conversion, including the two shapes a SET assignment cannot carry.
func TestDesignPropertyAssignment(t *testing.T) {
	theme := atlasListViewTheme()
	rowSize := &theme.WidgetProperties["ListView"][0]
	hover := &theme.WidgetProperties["ListView"][1]
	hideOn := &theme.WidgetProperties["Widget"][1]

	if vt, opt, err := designPropertyAssignment(rowSize, "Small"); err != nil || vt != "option" || opt != "Small" {
		t.Errorf("option value = (%q,%q,%v), want (option,Small,nil)", vt, opt, err)
	}
	if vt, _, err := designPropertyAssignment(hover, true); err != nil || vt != "toggle" {
		t.Errorf("toggle on = (%q,%v), want (toggle,nil)", vt, err)
	}
	// OFF is the absence of the entry, so it converts to no assignment and the
	// caller removes instead.
	if vt, _, err := designPropertyAssignment(hover, false); err != nil || vt != "" {
		t.Errorf("toggle off = (%q,%v), want ('',nil) so the caller removes", vt, err)
	}
	// Multi-select: refused, naming the spelling that works (ako/mxcli#511).
	_, _, err := designPropertyAssignment(hideOn, "Phone")
	if err == nil {
		t.Fatal("a flat value on a multi-select property was accepted; mxbuild refuses it with CE6084")
	}
	if !strings.Contains(err.Error(), "['Phone': on]") {
		t.Errorf("the message does not name the compound spelling: %v", err)
	}
}

// The keyword path and the stored-$Type path resolve through ONE key table: a
// keyword names the $Type it writes (mdlKeywordStorageType) and the key is read
// off that $Type (bsonTypeToDesignPropsKey). The two used to be separate
// hand-written keyword→key and $Type→key maps, and they disagreed — `groupbox`,
// `tabcontainer`, the menu widgets, `row`/`column`/`button` missing on one side,
// `radiobuttons`/`snippetcall`/`header`/`footer` naming keys no stored widget of
// that type has.
//
// What remains exclusive is one-directional and pinned: types MDL cannot build
// but Studio Pro can store, reachable only from a stored $Type.
//
//   - "DataGrid" is the NATIVE data grid. MDL's `datagrid` resolves through
//     pluggableKeywordIDs to Data grid 2's widget id; a Studio Pro-authored
//     Forms$DataGrid is reachable only from its $Type.
//   - "ReferenceSelector": there is no `referenceselector` builder (the keyword
//     parses and exec refuses it as an unsupported widget type).
func TestBsonTypeAndKeywordDesignPropsKeysAgree(t *testing.T) {
	fromKeyword := map[string]bool{}
	for kw := range mdlKeywordStorageType {
		key := resolveDesignPropsKey(kw)
		if _, ok := storageTypeThemeKeys[strings.TrimPrefix(mdlKeywordStorageType[kw], "Forms$")]; !ok {
			t.Errorf("keyword %q writes %s, which has no theme key", kw, mdlKeywordStorageType[kw])
		}
		fromKeyword[key] = true
	}
	fromBsonType := map[string]bool{}
	for _, v := range bsonTypeToDesignPropsKey {
		fromBsonType[v] = true
	}
	if len(fromKeyword) == 0 || len(fromBsonType) == 0 {
		t.Fatal("a map is empty — a passing run would prove nothing")
	}

	for k := range fromKeyword {
		if !fromBsonType[k] {
			t.Errorf("keyword-only key %q — a keyword resolved to a key no stored $Type has", k)
		}
	}
	storedOnly := map[string]bool{}
	for k := range fromBsonType {
		if !fromKeyword[k] {
			storedOnly[k] = true
		}
	}
	want := map[string]bool{"DataGrid": true, "ReferenceSelector": true}
	for k := range storedOnly {
		if !want[k] {
			t.Errorf("$Type-only key %q gained — if MDL cannot build that widget, add it here with the reason", k)
		}
	}
	for k := range want {
		if !storedOnly[k] {
			t.Errorf("$Type-only key %q lost — if a keyword now builds it, drop it from this list", k)
		}
	}
}

// The mutator accessor returns raw storage facts, not a resolved key — the
// layering that keeps the theme mapping in one place.
func TestWidgetStorageType(t *testing.T) {
	m := listViewMutator(t, storedListViewPage()).(interface {
		WidgetStorageType(string) (string, string)
	})
	bsonType, widgetID := m.WidgetStorageType("lvOrders")
	if bsonType != "Forms$ListView" {
		t.Errorf("bsonType = %q, want Forms$ListView", bsonType)
	}
	if widgetID != "" {
		t.Errorf("widgetID = %q, want empty for a native widget", widgetID)
	}
	if bt, wid := m.WidgetStorageType("nope"); bt != "" || wid != "" {
		t.Errorf("an unresolved widget returned (%q,%q), want both empty", bt, wid)
	}
}

var _ = bsonnav.DGetString
var _ = ast.SetPropertyOp{}

// The wiring, not just the decision: ALTER PAGE SET must reach
// SetDesignProperty, and the value must land in Appearance.DesignProperties
// rather than anywhere SetWidgetProperty would have put it.
func TestApplySetProperty_RoutesADesignProperty(t *testing.T) {
	stored := storedListViewPage()
	m := listViewMutator(t, stored)
	ctx, _ := newMockCtx(t)
	ctx.ThemeRegistry = atlasListViewTheme()

	op := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "lvOrders"},
		Properties: map[string]any{"Row size": "Small"},
	}
	if err := applySetPropertyMutator(ctx, m, op, "MyModule", model.ID("mod")); err != nil {
		t.Fatalf("set: %v", err)
	}

	entries := storedDesignProperties(t, stored, "lvOrders")
	if len(entries) != 1 {
		t.Fatalf("got %d design property entries, want 1", len(entries))
	}
	if got := bsonnav.DGetString(entries[0], "Key"); got != "Row size" {
		t.Errorf("Key = %q, want \"Row size\"", got)
	}
}

// A toggle set to OFF is a REMOVAL, not a stored false: Mendix represents the
// off state as the absence of the entry, which is what `alter styling … = off`
// already does. Writing a false toggle leaves an entry Studio Pro shows as on.
func TestApplySetProperty_ToggleOffRemoves(t *testing.T) {
	stored := storedListViewPage()
	m := listViewMutator(t, stored)
	ctx, _ := newMockCtx(t)
	ctx.ThemeRegistry = atlasListViewTheme()

	on := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "lvOrders"},
		Properties: map[string]any{"Hover style": true},
	}
	if err := applySetPropertyMutator(ctx, m, on, "MyModule", model.ID("mod")); err != nil {
		t.Fatalf("set on: %v", err)
	}
	if n := len(storedDesignProperties(t, stored, "lvOrders")); n != 1 {
		t.Fatalf("after ON: %d entries, want 1", n)
	}

	off := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "lvOrders"},
		Properties: map[string]any{"Hover style": false},
	}
	if err := applySetPropertyMutator(ctx, m, off, "MyModule", model.ID("mod")); err != nil {
		t.Fatalf("set off: %v", err)
	}
	if n := len(storedDesignProperties(t, stored, "lvOrders")); n != 0 {
		t.Errorf("after OFF: %d entries, want 0 — off is the absence of the entry", n)
	}
}

// The control for the routing: a key the theme does NOT declare for this widget
// must stay on the pluggable path and keep that path's error, or a typo becomes
// a silently-written design property.
func TestApplySetProperty_UnknownKeyIsNotRoutedAsDesignProperty(t *testing.T) {
	stored := storedListViewPage()
	m := listViewMutator(t, stored)
	ctx, _ := newMockCtx(t)
	ctx.ThemeRegistry = atlasListViewTheme()

	op := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "lvOrders"},
		Properties: map[string]any{"Rowsize": "Small"},
	}
	err := applySetPropertyMutator(ctx, m, op, "MyModule", model.ID("mod"))
	if err == nil {
		t.Fatal("a mistyped key was accepted")
	}
	if n := len(storedDesignProperties(t, stored, "lvOrders")); n != 0 {
		t.Errorf("a mistyped key wrote %d design property entry(ies)", n)
	}
}

// storedDesignProperties returns the widget's DesignProperties entries, marker
// stripped.
func storedDesignProperties(t *testing.T, stored bson.D, widgetName string) []bson.D {
	t.Helper()
	var out []bson.D
	var walk func(v any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if bsonnav.DGetString(d, "Name") == widgetName {
				if app := bsonnav.DGetDoc(d, "Appearance"); app != nil {
					for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(app, "DesignProperties")) {
						if ed, ok := e.(bson.D); ok {
							out = append(out, ed)
						}
					}
				}
			}
			for _, e := range d {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	walk(stored)
	return out
}
