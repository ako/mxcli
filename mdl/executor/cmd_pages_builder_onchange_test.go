// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mkOnChangeWidget builds a V3 widget of the given MDL type carrying an
// `OnChange:` action. `close` is used because it resolves without touching the
// backend — the point under test is whether the property survives the builder,
// not which action it is.
func mkOnChangeWidget(mdlType, name string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: mdlType,
		Name: name,
		Properties: map[string]any{
			"Attribute": "Value",
			"Label":     "L",
			"OnChange":  &ast.ActionV3{Type: "close"},
		},
	}
}

// onChangeOf returns the OnChangeAction of any input widget that has one.
func onChangeOf(t *testing.T, w pages.Widget) pages.ClientAction {
	t.Helper()
	switch x := w.(type) {
	case *pages.TextBox:
		return x.OnChangeAction
	case *pages.TextArea:
		return x.OnChangeAction
	case *pages.DatePicker:
		return x.OnChangeAction
	case *pages.DropDown:
		return x.OnChangeAction
	case *pages.CheckBox:
		return x.OnChangeAction
	case *pages.RadioButtons:
		return x.OnChangeAction
	default:
		t.Fatalf("widget type %T has no OnChangeAction field", w)
		return nil
	}
}

// TestBuildWidgetV3_OnChangeSurvivesBuilder covers ledger #14: an `OnChange:`
// authored on checkbox / radiobuttons / dropdown / textarea / datepicker parsed
// and executed without error, but the builder never read it — the property was
// dropped between the AST and the model, so the rendered control produced no
// server round-trip at all. Only `textbox` read it.
func TestBuildWidgetV3_OnChangeSurvivesBuilder(t *testing.T) {
	for _, mdlType := range []string{
		"textbox", "textarea", "datepicker", "dropdown", "checkbox", "radiobuttons",
	} {
		t.Run(mdlType, func(t *testing.T) {
			mod := mkModule("Mod")
			h := mkHierarchy(mod)
			withContainer(h, mod.ID, mod.ID)
			pb := newPageBuilder(&mock.MockBackend{}, h, "Mod")

			w, err := pb.buildWidgetV3(mkOnChangeWidget(mdlType, "w1"))
			if err != nil {
				t.Fatalf("buildWidgetV3(%s): %v", mdlType, err)
			}
			if act := onChangeOf(t, w); act == nil {
				t.Fatalf("%s: OnChange was dropped by the builder (OnChangeAction is nil)", mdlType)
			}
		})
	}
}
