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
