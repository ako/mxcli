// SPDX-License-Identifier: Apache-2.0

// The PARAMETERS of a text-template sub-property on an object-list item.
//
// A template carrying `{1}` with no parameter is CE0720 ("Place holder index 1
// is greater than 0, the number of parameter(s)"), so describing the text
// without its parameters turns a valid page into one that fails the build.
// Measured on a Studio Pro-authored File Uploader 2.5.0 custom button, whose
// caption is `This is a custom button {1}`: the page went from 1 error to 2, and
// the DESCRIBE text was identical either way (#956).
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
)

// DESCRIBE must emit the companion `<Name>Params` beside the template text.
func TestExtractObjectListItem_EmitsTextTemplateParameters(t *testing.T) {
	const idCaption = "id-caption"
	nested := map[string]string{idCaption: "buttonCaption"}
	itemObj := map[string]any{"Properties": []any{
		int32(2), // BSON non-empty-array marker
		map[string]any{"TypePointer": idCaption, "Value": map[string]any{
			"TextTemplate": map[string]any{
				"Template": map[string]any{
					"Items": []any{int32(2), map[string]any{"Text": "Hello {1}"}},
				},
				"Parameters": []any{int32(2), map[string]any{
					"$Type":      "Forms$ClientTemplateParameter",
					"Expression": "'abc'",
				}},
			},
		}},
	}}

	item := extractObjectListItem(nil, itemObj, nested)

	var text, params string
	for _, p := range item.Props {
		switch p.Key {
		case "ButtonCaption":
			text = p.Value
		case "ButtonCaptionParams":
			params = p.Value
			if !p.IsRef {
				t.Error("the parameter list was quoted — `ButtonCaptionParams: '[...]'` does not parse")
			}
		}
	}
	if text == "" {
		t.Fatalf("template text missing; props = %v", item.Props)
	}
	if params == "" {
		t.Fatalf("template PARAMETERS missing — re-executing this description leaves {1} with no parameter (CE0720); props = %v", item.Props)
	}
	// paramListV3 is `[{N} = expr]`; a bare expression list does not parse.
	if !strings.HasPrefix(params, "[{1} = ") {
		t.Errorf("params = %q, want the [{N} = expr] form paramListV3 accepts", params)
	}
}

// The engine reads the companion back. matchedAlias is the SCHEMA key
// (`buttonCaption`) while the script writes the widget's own casing, so the
// lookup has to be case-insensitive like every other MDL property name — a
// direct map index agreed only for a DataGrid column's `CaptionParams`, which is
// why this went unnoticed.
func TestBuildObjectListItem_ReadsParamsCompanionCaseInsensitively(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	mapping := &ObjectListMapping{
		PropertyKey:  "customButtons",
		MDLContainer: "CUSTOMBUTTON",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "buttonCaption", Operation: "texttemplate"},
		},
	}
	child := &ast.WidgetV3{Name: "b1", Properties: map[string]any{
		"ButtonCaption":       "Hello {1}",
		"ButtonCaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "'abc'"}},
	}}

	spec, err := e.buildObjectListItem(mapping, child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}
	var prop *backend.ObjectListItemProperty
	for i := range spec.Properties {
		if spec.Properties[i].PropertyKey == "buttonCaption" {
			prop = &spec.Properties[i]
		}
	}
	if prop == nil {
		t.Fatalf("no buttonCaption property in the item spec; got %d", len(spec.Properties))
	}
	if len(prop.Parameters) == 0 {
		t.Fatal("the params companion was not read — the template ships {1} with no parameter and the build fails CE0720")
	}
}
