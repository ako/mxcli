// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// TestActionSourceForKey_EventSuffixedSlots covers the pluggable half of ledger
// #14. `actionSourceForKey` matched only the bare keys `onClick` / `onChange`,
// but Mendix's own pluggable widgets suffix their action slots: the Combobox
// names its on-change slot `onChangeEvent`. No mapping was emitted for it, so an
// `OnChange:` authored on a `combobox` was dropped with no error — the same
// silent drop as the built-in input widgets, one layer down.
//
// The Combobox's other change-shaped slots (`onChangeFilterInputEvent`,
// `onChangeDatabaseEvent`) are distinct properties with no MDL surface and must
// stay unmapped, or `OnChange:` would write three different actions.
func TestActionSourceForKey_EventSuffixedSlots(t *testing.T) {
	cases := map[string]string{
		"onChange":                 "OnChange",
		"onChangeEvent":            "OnChange",
		"onChangeAction":           "OnChange",
		"onClick":                  "OnClick",
		"onClickEvent":             "OnClick",
		"onChangeFilterInputEvent": "",
		"onChangeDatabaseEvent":    "",
		"onEnterEvent":             "",
		"onSelectionChange":        "",
	}
	for key, want := range cases {
		if got := actionSourceForKey(key); got != want {
			t.Errorf("actionSourceForKey(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestGenerateDefJSON_ComboboxOnChangeEvent asserts the mapping actually reaches
// a generated definition for a Combobox-shaped MPK, not just the key matcher.
func TestGenerateDefJSON_ComboboxOnChangeEvent(t *testing.T) {
	def := GenerateDefJSON(&mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.combobox.Combobox",
		Name: "Combo box",
		Properties: []mpk.PropertyDef{
			{Key: "attributeEnumeration", Type: "attribute"},
			{Key: "onChangeEvent", Type: "action"},
			{Key: "onChangeFilterInputEvent", Type: "action"},
			{Key: "onEnterEvent", Type: "action"},
		},
	}, "COMBOBOX")

	var mapped []string
	for _, m := range def.PropertyMappings {
		if m.Operation == "action" {
			mapped = append(mapped, m.PropertyKey+"→"+m.Source)
		}
	}
	if len(mapped) != 1 || mapped[0] != "onChangeEvent→OnChange" {
		t.Fatalf("action mappings = %v, want exactly [onChangeEvent→OnChange]", mapped)
	}
}

// TestCombobox_OnChangeMappedInBothModes pins the shipped built-in definition,
// which is hand-written and therefore not covered by the generator fix above:
// the built-in def overrides any .mpk-derived one, so without a mapping here the
// generator change would never reach a real combobox. Modes are exclusive, so
// the mapping has to be present in each — the reported drop (ledger #14) was on
// an enumeration-bound combobox, i.e. the default mode.
func TestCombobox_OnChangeMappedInBothModes(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	def, ok := reg.Get("COMBOBOX")
	if !ok {
		t.Fatal("no COMBOBOX definition in the built-in registry")
	}
	engine := &PluggableWidgetEngine{}

	widgets := map[string]*ast.WidgetV3{
		"enumeration": {Name: "cb", Type: "combobox", Properties: map[string]any{
			"Attribute": "TopicSel",
			"OnChange":  &ast.ActionV3{Type: "close"},
		}},
		"association": {Name: "cb", Type: "combobox", Properties: map[string]any{
			"Association":      "Mod.A_B",
			"CaptionAttribute": "Name",
			"DataSource":       &ast.DataSourceV3{Type: "database", Reference: "Mod.B"},
			"OnChange":         &ast.ActionV3{Type: "close"},
		}},
	}

	for mode, w := range widgets {
		t.Run(mode, func(t *testing.T) {
			mappings, _, err := engine.selectMappings(def, w)
			if err != nil {
				t.Fatalf("selectMappings: %v", err)
			}
			var onChange *PropertyMapping
			for i := range mappings {
				if mappings[i].Operation == "action" && mappings[i].Source == "OnChange" {
					onChange = &mappings[i]
				}
			}
			if onChange == nil {
				t.Fatal("no OnChange action mapping — an OnChange: on a combobox is dropped silently")
			}
			if onChange.PropertyKey != "onChangeEvent" {
				t.Fatalf("OnChange maps to %q, want the Combobox's own slot \"onChangeEvent\"", onChange.PropertyKey)
			}

			// The mapping is only half of it: resolveMapping must actually build the
			// client action out of the AST, or the engine writes a nil action.
			mod := mkModule("Mod")
			h := mkHierarchy(mod)
			withContainer(h, mod.ID, mod.ID)
			engine.pageBuilder = newPageBuilder(&mock.MockBackend{}, h, "Mod")
			ctx, err := engine.resolveMapping(*onChange, w)
			if err != nil {
				t.Fatalf("resolveMapping: %v", err)
			}
			if ctx.Action == nil {
				t.Fatal("resolveMapping produced no client action for OnChange")
			}
		})
	}
}

// TestReadsFixedASTSlot_MeasuredOperations pins the exclusion list to what was
// measured on a real Combobox, not to the shape of the mapping.
//
// The tempting rule — "any mapping whose PropertyKey differs from its Source" —
// is wrong and would reject working syntax: on Mendix 11.13,
// `optionsSourceAssociationCaptionAttribute:` persists even though its MDL name
// is `CaptionAttribute`, while `attributeAssociation:` and `onChangeEvent:` do
// not. Only the two operations that resolve from a dedicated AST accessor are
// excluded.
func TestReadsFixedASTSlot_MeasuredOperations(t *testing.T) {
	excluded := map[string]bool{"action": true, "association": true}
	for _, op := range []string{
		"action", "association",
		"attribute", "primitive", "datasource", "texttemplate",
		"selection", "widgets", "attributeObjects", "expression",
	} {
		if got := readsFixedASTSlot(op); got != excluded[op] {
			t.Errorf("readsFixedASTSlot(%q) = %v, want %v — "+
				"an operation belongs here only once the written document shows "+
				"its storage key is dropped", op, got, excluded[op])
		}
	}
}
