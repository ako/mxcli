// SPDX-License-Identifier: Apache-2.0

// The MAPPING pass must route `contentparams:` the way the explicit-property
// pass has since #928. A `{1}`-style template written with an empty parameter
// list is CE0720 ("Place holder index 1 is greater than 0, the number of
// parameter(s)").
//
// This was unreachable while such text was being dropped altogether — the Slider
// tooltip of #254 — so fixing the drop is what exposed it.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func tooltipDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.custom.slider.Slider",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "tooltip", Source: "TextTemplate", Operation: "texttemplate"},
		},
	}
}

func TestResolveMapping_TextTemplateCarriesContentParamsForANumericPlaceholder(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "TN.Reading"}, currentDef: tooltipDef()}
	w := &ast.WidgetV3{Name: "sl", Properties: map[string]any{
		"tooltip": "Score is {1}",
	}}
	w.Properties["ContentParams"] = []ast.ParamAssignmentV3{{Index: 1, Value: "Score"}}

	ctx, err := e.resolveMapping(tooltipDef().PropertyMappings[0], w)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.PrimitiveVal != "Score is {1}" {
		t.Fatalf("PrimitiveVal = %q", ctx.PrimitiveVal)
	}
	if len(ctx.ClientParams) != 1 {
		t.Fatalf("ClientParams = %d, want 1 — a {1} written with no parameter is CE0720", len(ctx.ClientParams))
	}
}

// Text with no numeric placeholder must not pick up parameters, so the ordinary
// caption path is untouched.
func TestResolveMapping_TextTemplateWithoutPlaceholderTakesNoParams(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "TN.Reading"}, currentDef: tooltipDef()}
	w := &ast.WidgetV3{Name: "sl", Properties: map[string]any{"tooltip": "Current score"}}
	w.Properties["ContentParams"] = []ast.ParamAssignmentV3{{Index: 1, Value: "Score"}}

	ctx, err := e.resolveMapping(tooltipDef().PropertyMappings[0], w)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ClientParams) != 0 {
		t.Errorf("ClientParams = %d, want 0 for placeholder-less text", len(ctx.ClientParams))
	}
}
