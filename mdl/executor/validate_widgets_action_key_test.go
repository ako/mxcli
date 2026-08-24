// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A widget's action slot is addressed in MDL by the engine's source name
// (`OnChange:` / `Action:` / `OnClick:`), never by the widget's own storage key.
// `resolveMapping` reads `w.GetOnChange()`, so `onChangeEvent: …` lands nowhere:
// it would be accepted by check and dropped on write — the exact failure class
// FINDINGS #14 was about, one layer over.
//
// Making `onChangeEvent` a mapped property (so `OnChange:` reaches the Combobox
// at all) put its storage key into the allowed set as a side effect. This pins
// the split: the source name is authorable, the storage key is not, and writing
// the storage key gets a message that names the spelling that works.
func TestValidatePluggableWidgetProperties_ActionStorageKeyIsNotAuthorable(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}

	violationsFor := func(props map[string]any) []linter.Violation {
		w := &ast.WidgetV3{Name: "cmbX", Type: "combobox", Properties: props}
		return validatePluggableWidgetProperties(w, reg, "page M.P")
	}

	// The storage key must be rejected, with a hint naming `OnChange`.
	got := violationsFor(map[string]any{
		"Attribute":     "Name",
		"onChangeEvent": "nope",
	})
	var found *linter.Violation
	for i := range got {
		if got[i].RuleID == "MDL-WIDGET01" {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("onChangeEvent must not be authorable — it is read from nowhere; got %v", got)
	}
	if found.Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error", found.Severity)
	}
	if !strings.Contains(found.Message, "OnChange") {
		t.Errorf("message must name the MDL spelling `OnChange`:\n%s", found.Message)
	}

	// The source name is the real spelling and must stay clean.
	for _, v := range violationsFor(map[string]any{
		"Attribute": "Name",
		"OnChange":  &ast.ActionV3{Type: "close"},
	}) {
		if v.Severity == linter.SeverityError {
			t.Errorf("`OnChange:` on a combobox produced an error: %s %s", v.RuleID, v.Message)
		}
	}
}

// Some widgets spell their action slot exactly as MDL does. Slider, RangeSlider
// and StarRating all store `onChange`, and the storage-key check above then fired
// against MDL's own `OnChange:` — telling the author to "use `OnChange:` instead"
// of `OnChange:`, and leaving those widgets' only action slot unauthorable.
//
// The cause was that `OnChange` was missing from isBuiltinPropName while `Action`
// was there, so it never took the early exit the other dedicated keywords take.
// (upstream #956)
func TestValidatePluggableWidgetProperties_MDLNameIsNotMistakenForTheStorageKey(t *testing.T) {
	// A Slider-shaped def: its action slot's storage key IS `onChange`.
	def := &WidgetDefinition{
		WidgetID: "com.mendix.widget.custom.slider.Slider",
		MDLName:  "SLIDER",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "valueAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "onChange", Source: "OnChange", Operation: "action"},
		},
	}
	reg := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"SLIDER": def}}

	w := &ast.WidgetV3{Name: "sl", Type: "slider", Properties: map[string]any{
		"Attribute": "Score",
		"OnChange":  &ast.ActionV3{Type: "close"},
	}}

	for _, v := range validatePluggableWidgetProperties(w, reg, "page M.P") {
		if v.Severity == linter.SeverityError {
			t.Errorf("`OnChange:` rejected on a widget whose storage key is also `onChange` — "+
				"this is MDL's own spelling and the only way to author the slot:\n  [%s] %s",
				v.RuleID, v.Message)
		}
	}
}

// Both dedicated action keywords take the same route, so neither should ever be
// mistaken for a storage key. Asserted together because the bug was one of them
// being present in isBuiltinPropName and the other missing.
func TestBuiltinPropNamesCoverBothActionKeywords(t *testing.T) {
	for _, name := range []string{"Action", "OnChange"} {
		if !isBuiltinPropName(name) {
			t.Errorf("%q is a dedicated MDL keyword resolved by resolveMapping, not a widget "+
				"storage key — omitting it makes the validator reject MDL's own spelling", name)
		}
	}
}
