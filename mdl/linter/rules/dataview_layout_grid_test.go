// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

func textbox(name string) map[string]any {
	return map[string]any{"$Type": "Forms$TextBox", "Name": name}
}

// dataView builds a DataView BSON node with the given direct child widgets. The
// data source is irrelevant to the rule, so it's omitted.
func dataView(name string, children ...any) map[string]any {
	return map[string]any{"$Type": "Forms$DataView", "Name": name, "Widgets": children}
}

func TestIsFormDataView(t *testing.T) {
	// A DataView with an input is a form (regardless of data source).
	if !isFormDataView(dataView("dv", textbox("tb"))) {
		t.Error("DataView with a textbox should be a form")
	}
	// An input nested in a container still counts.
	nested := dataView("dv", map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{textbox("tb")}})
	if !isFormDataView(nested) {
		t.Error("DataView with an input inside a container should be a form")
	}
	// The pluggable ComboBox counts.
	combo := map[string]any{"$Type": "CustomWidgets$CustomWidget", "Type": map[string]any{"WidgetId": "com.mendix.widget.web.combobox.Combobox"}}
	if !isFormDataView(dataView("dv", combo)) {
		t.Error("DataView with a ComboBox should be a form")
	}
	// Display-only DataView (no inputs) is NOT a form.
	display := dataView("dv", map[string]any{"$Type": "Forms$DynamicText", "Name": "t"})
	if isFormDataView(display) {
		t.Error("display-only DataView should not be a form")
	}
	// Container DataView wrapping a nested datagrid (no inputs) is NOT a form.
	wrapper := dataView("dv", map[string]any{"$Type": "Forms$DataGrid", "Name": "g"})
	if isFormDataView(wrapper) {
		t.Error("DataView wrapping a datagrid should not be a form")
	}
	// An input that lives in a *nested* DataView must not make the outer one a form.
	outer := dataView("outer", dataView("inner", textbox("tb")))
	if isFormDataView(outer) {
		t.Error("input in a nested DataView should not count for the outer DataView")
	}
	// Non-DataView is never a form.
	if isFormDataView(map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{textbox("tb")}}) {
		t.Error("non-DataView should not be a form")
	}
}

// collectReported runs the walk and returns the names of flagged DataViews.
func collectReported(root map[string]any) []string {
	var names []string
	walkForUngridedDataView(root, false, func(dv map[string]any) {
		names = append(names, widgetName(dv))
	})
	return names
}

func TestWalkForUngridedDataView_FlagsBareForm(t *testing.T) {
	root := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{dataView("dvBare", textbox("tb"))}}
	got := collectReported(root)
	if len(got) != 1 || got[0] != "dvBare" {
		t.Fatalf("expected [dvBare], got %v", got)
	}
}

// The data source is irrelevant: a database-bound form DataView outside a grid is
// flagged just like a parameter-bound one.
func TestWalkForUngridedDataView_DatabaseFormFlagged(t *testing.T) {
	dv := dataView("dvDb", textbox("tb"))
	dv["DataSource"] = map[string]any{"$Type": "Forms$DatabaseSource"}
	root := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{dv}}
	if got := collectReported(root); len(got) != 1 || got[0] != "dvDb" {
		t.Fatalf("expected [dvDb], got %v", got)
	}
}

func TestWalkForUngridedDataView_GridWrappedIsClean(t *testing.T) {
	root := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Rows": []any{map[string]any{
			"Columns": []any{map[string]any{"Widgets": []any{dataView("dvWrapped", textbox("tb"))}}},
		}},
	}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("grid-wrapped form should not be flagged, got %v", got)
	}
}

func TestWalkForUngridedDataView_GridAncestorThroughContainer(t *testing.T) {
	root := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Rows": []any{map[string]any{
			"Columns": []any{map[string]any{"Widgets": []any{map[string]any{
				"$Type":   "Forms$DivContainer",
				"Widgets": []any{dataView("dvNested", textbox("tb"))},
			}}}},
		}},
	}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("DataView under a grid ancestor should not be flagged, got %v", got)
	}
}

// A display-only DataView outside a grid has no label/input-width concern.
func TestWalkForUngridedDataView_DisplayOnlyIgnored(t *testing.T) {
	dv := dataView("dvDisplay", map[string]any{"$Type": "Forms$DynamicText", "Name": "t"})
	root := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{dv}}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("display-only DataView should not be flagged, got %v", got)
	}
}

// rawUnitReader serves raw units by ID; everything else is empty.
type rawUnitReader struct{ units map[model.ID]map[string]any }

func (r rawUnitReader) GetMicroflow(model.ID) (*microflows.Microflow, error) { return nil, nil }
func (r rawUnitReader) ListMicroflows() ([]*microflows.Microflow, error)     { return nil, nil }
func (r rawUnitReader) GetProjectSecurity() (*security.ProjectSecurity, error) {
	return nil, nil
}
func (r rawUnitReader) GetProjectSettings() (*model.ProjectSettings, error) { return nil, nil }
func (r rawUnitReader) GetNavigation() (*types.NavigationDocument, error)   { return nil, nil }
func (r rawUnitReader) ListPages() ([]*pages.Page, error)                   { return nil, nil }
func (r rawUnitReader) ListModules() ([]*model.Module, error)               { return nil, nil }
func (r rawUnitReader) ListFolders() ([]*types.FolderInfo, error)           { return nil, nil }
func (r rawUnitReader) GetRawUnit(id model.ID) (map[string]any, error)      { return r.units[id], nil }
func (r rawUnitReader) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	return nil, nil
}

// ako/mxcli#962 item 4: the layout-grid advice is web-only. Measured on mxbuild
// 11.13.0 (PedApp copy): a form DataView directly on an
// Atlas_Core.NativePhone_Default page, or in a snippet of Type Native, builds
// clean, and wrapping it in a layoutgrid there is CE6858. The web page and the
// web snippet are the controls.
func TestDataViewLayoutGridRule_SkipsNativePagesAndSnippets(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, stmt := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES
			('m1', 'MyFirstModule', 'MyFirstModule', 'MyFirstModule', ''),
			('m2', 'Atlas_Core', 'Atlas_Core', 'Atlas_Core', 'Atlas_Core.mpk')`,
		`INSERT INTO layouts_data (Id, Name, QualifiedName, ModuleName, LayoutType, Platform) VALUES
			('l1', 'Atlas_Default', 'Atlas_Core.Atlas_Default', 'Atlas_Core', 'Responsive', 'Web'),
			('l2', 'NativePhone_Default', 'Atlas_Core.NativePhone_Default', 'Atlas_Core', 'Default', 'Native')`,
		`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, LayoutRef) VALUES
			('p1', 'P_WebForm', 'MyFirstModule.P_WebForm', 'MyFirstModule', 'Atlas_Core.Atlas_Default'),
			('p2', 'P_NativeForm', 'MyFirstModule.P_NativeForm', 'MyFirstModule', 'Atlas_Core.NativePhone_Default')`,
		`INSERT INTO snippets_data (Id, Name, QualifiedName, ModuleName) VALUES
			('s1', 'SN_Web', 'MyFirstModule.SN_Web', 'MyFirstModule'),
			('s2', 'SN_Native', 'MyFirstModule.SN_Native', 'MyFirstModule')`,
	} {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	page := func(dv string) map[string]any {
		return map[string]any{"FormCall": map[string]any{"Arguments": []any{
			map[string]any{"Widgets": []any{dataView(dv, textbox("tb"))}}}}}
	}
	snippet := func(typ, dv string) map[string]any {
		return map[string]any{"Type": typ, "Widgets": []any{dataView(dv, textbox("tb"))}}
	}
	reader := rawUnitReader{units: map[model.ID]map[string]any{
		"p1": page("dvWeb"), "p2": page("dvNative"),
		"s1": snippet("Web", "dvSnipWeb"), "s2": snippet("Native", "dvSnipNative"),
	}}
	var got []string
	for _, v := range NewDataViewLayoutGridRule().Check(linter.NewLintContext(cat, reader)) {
		got = append(got, v.Location.DocumentName)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "P_WebForm,SN_Web" {
		t.Fatalf("reported %v, want only the web page and the web snippet", got)
	}
}
