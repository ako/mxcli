// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `DataSource: Module.Entity` — the bare-entity shorthand — did not match
// dataSourceExprV3 (every alternative there starts with a keyword or a
// variable), so it fell through to the generic `keyword COLON propertyValueV3`
// branch and landed in Properties["DataSource"] as a plain string. Nothing
// downstream reads a string there: `check` passed, `exec` reported "Created
// page", DESCRIBE showed the grid with no source, and mxbuild reported CE0488
// "No entity configured for the data source of this widgets container"
// (ako/mxcli#576). The shorthand now means what a reader expects: DATABASE.
func TestDataSourceBareEntityShorthand_BindsDatabase(t *testing.T) {
	prog, errs := Build(`create page Mod.Shorthand (Title: 'Cars', Layout: Atlas_Core.Atlas_Default) {
  datagrid dgShort (DataSource: Mod.Car, onClick: SHOW_PAGE Mod.Detail(Car: $currentObject)) {
    column colName (Caption: 'Name', Attribute: Name)
  }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	w := findWidget(page.Widgets, "dgShort")
	if w == nil {
		t.Fatal("datagrid dgShort not found")
	}
	ds, ok := w.Properties["DataSource"].(*ast.DataSourceV3)
	if !ok || ds == nil {
		t.Fatalf("DataSource = %#v (%T), want *ast.DataSourceV3 — the shorthand was dropped",
			w.Properties["DataSource"], w.Properties["DataSource"])
	}
	if ds.Type != "database" || ds.Reference != "Mod.Car" {
		t.Errorf("DataSource = {Type: %q, Reference: %q}, want {database, Mod.Car}", ds.Type, ds.Reference)
	}
}

// A pluggable widget's generic keys are its own .mpk property keys, and two
// reuse the name: the Barcode Scanner's `datasource` binds an ATTRIBUTE
// (mendixlabs/mxcli#1161 writes it as `Module.Entity.Code`), and describe emits
// it back as the bare `datasource: Code`. Reading either as an entity broke the
// scanner — the first cut of this fix did exactly that.
func TestDataSourceBareEntityShorthand_PluggableKeyUntouched(t *testing.T) {
	prog, errs := Build(`create page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default, Params: { $Scan: Mod.Scan }) {
  dataview dv (DataSource: $Scan) {
    barcodescanner bs1 (datasource: Mod.Scan.Code)
    barcodescanner bs2 (datasource: Code)
  }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	dv := findWidget(page.Widgets, "dv")
	for name, want := range map[string]string{"bs1": "Mod.Scan.Code", "bs2": "Code"} {
		var w *ast.WidgetV3
		for _, c := range dv.Children {
			if c.Name == name {
				w = c
			}
		}
		if w == nil {
			t.Fatalf("%s not found", name)
		}
		if got := w.Properties["datasource"]; got != want {
			t.Errorf("%s: Properties[datasource] = %#v, want the attribute string %q", name, got, want)
		}
		if w.GetDataSource() != nil {
			t.Errorf("%s: attribute key was read as a data source: %+v", name, w.GetDataSource())
		}
	}
}

// The explicit forms must keep their meaning — the shorthand must not capture
// a variable or a keyword-led source.
func TestDataSourceBareEntityShorthand_ExplicitFormsUnchanged(t *testing.T) {
	prog, errs := Build(`create page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default, Params: { $Car: Mod.Car }) {
  datagrid g1 (DataSource: DATABASE Mod.Car) { column c (Attribute: Name) }
  dataview dv (DataSource: $Car) { textbox t (Attribute: Name) }
  datagrid g2 (DataSource: MICROFLOW Mod.DS_Cars) { column c2 (Attribute: Name) }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	for name, want := range map[string]string{"g1": "database", "dv": "parameter", "g2": "microflow"} {
		ds, ok := findWidget(page.Widgets, name).Properties["DataSource"].(*ast.DataSourceV3)
		if !ok || ds.Type != want {
			t.Errorf("%s: DataSource = %#v, want type %q", name, ds, want)
		}
	}
}
