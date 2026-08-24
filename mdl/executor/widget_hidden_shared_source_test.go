// SPDX-License-Identifier: Apache-2.0

// An MDL keyword shared by several widget properties, where the widget's
// configuration hides some of them.
//
// File Uploader 2.5.0 routes BOTH `associatedFiles` and `associatedImages` from
// `DataSource:`, and `uploadMode` decides which one Studio Pro shows. A script
// writing `DataSource:` once has named the keyword, not the hidden property — so
// the hidden one must keep the widget template's default rather than receive the
// value. Measured on a Studio Pro-authored widget: all 34 declared properties are
// stored, the hidden ones at their default, so "hidden" means default-valued,
// not absent (#956).
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func fileUploaderLikeDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.example.TwoSourceWidget",
		MDLName:  "TWOSOURCE",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "uploadMode", Operation: "primitive"},
			{PropertyKey: "associatedFiles", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "associatedImages", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "onlyMine", Source: "Caption", Operation: "primitive"},
		},
	}
}

// The user wrote `DataSource:` once — that does not name either property, so
// neither is "explicitly set" for MDL-WIDGET10's purposes.
func TestWidgetValueMap_SharedSourceDoesNotNameAProperty(t *testing.T) {
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{
		"uploadMode": "files",
		"DataSource": "whatever",
		"Caption":    "hi",
	}}

	values, explicit := widgetValueMap(w, fileUploaderLikeDef())

	if explicit["associatedfiles"] || explicit["associatedimages"] {
		t.Error("a keyword shared by two properties was treated as naming them; MDL-WIDGET10 then fires on a property the script never mentioned")
	}
	if !explicit["onlymine"] {
		t.Error("a source unique to ONE property still names it — that is the DataGrid2 onSelectionChange case and must keep working")
	}
	if values["uploadmode"] != "files" {
		t.Errorf("the value must still be recorded (conditions read it), got %q", values["uploadmode"])
	}
}
