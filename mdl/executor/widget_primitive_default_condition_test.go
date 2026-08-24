// SPDX-License-Identifier: Apache-2.0

// A visibility rule keyed on an enumeration the script did not name must still
// fire: the widget XML declares a defaultValue, the builder WRITES that value, so
// the condition is determinable and the property it guards is genuinely pruned.
//
// File Uploader 2.5.0 is the case. `uploadMode` defaults to "files", which hides
// `associatedImages`; both datasource properties are reached through the one
// `DataSource:` clause, so with the condition unknown the clause fanned out into
// both and the pruned one carried a value — CE0463 on every page with the widget
// (#956). Measured on 11.13.0: 1 error before, 0 after, and `mx update-widgets`
// (Studio Pro's normalizer) clears the same error by nulling exactly that second
// datasource.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func uploadModeDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.fileuploader.FileUploader",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "uploadMode", Operation: "primitive", Value: "files"},
			{PropertyKey: "associatedFiles", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "associatedImages", Source: "DataSource", Operation: "datasource"},
		},
		PropertyVisibility: []types.WidgetVisibilityRule{
			{PropertyKey: "associatedImages", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "uploadMode", Operator: "eq", Value: "files"}},
			{PropertyKey: "associatedFiles", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "uploadMode", Operator: "ne", Value: "files"}},
		},
	}
}

func TestWidgetValueMap_UnnamedPrimitiveFallsBackToItsDeclaredDefault(t *testing.T) {
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{"DataSource": "assoc"}}

	values, _ := widgetValueMap(w, uploadModeDef())

	if got := values["uploadmode"]; got != "files" {
		t.Fatalf("uploadMode = %q, want %q — the script named nothing, so the "+
			"builder writes the XML default and every rule keyed on it must fire", got, "files")
	}
}

// The script's own value still wins over the default.
func TestWidgetValueMap_NamedPrimitiveBeatsTheDefault(t *testing.T) {
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{
		"uploadMode": "images", "DataSource": "assoc"}}

	values, _ := widgetValueMap(w, uploadModeDef())

	if got := values["uploadmode"]; got != "images" {
		t.Fatalf("uploadMode = %q, want %q", got, "images")
	}
}

// End of the chain: with the condition now determinable, the pruned datasource is
// the one that gets skipped — and it flips with the mode.
func TestHiddenUnnamedProperties_PrunesTheInactiveDataSource(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := uploadModeDef()

	filesMode := e.hiddenUnnamedProperties(def, &ast.WidgetV3{
		Name: "fu", Properties: map[string]any{"DataSource": "assoc"}})
	if !filesMode["associatedimages"] {
		t.Error("associatedImages was not pruned under the default uploadMode — this is the CE0463")
	}
	if filesMode["associatedfiles"] {
		t.Error("associatedFiles was pruned under uploadMode files — the widget would lose its data source")
	}

	imagesMode := e.hiddenUnnamedProperties(def, &ast.WidgetV3{
		Name: "fu", Properties: map[string]any{"uploadMode": "images", "DataSource": "assoc"}})
	if !imagesMode["associatedfiles"] {
		t.Error("associatedFiles was not pruned under uploadMode images")
	}
	if imagesMode["associatedimages"] {
		t.Error("associatedImages was pruned under uploadMode images — CE0642, the property is required there")
	}
}
