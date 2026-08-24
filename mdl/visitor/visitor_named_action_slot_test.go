// SPDX-License-Identifier: Apache-2.0

// A named action slot is written as an ordinary widget property whose value is
// an action: `createFileAction: show_page Module.P`. The grammar carries the
// forms that do not also parse as a data source; MICROFLOW / NANOFLOW / VARIABLE
// overlap with dataSourceExprV3 and are converted by the executor, which is the
// layer that knows the slot is action-typed. (upstream #956)
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func namedSlotProp(t *testing.T, prop string) any {
	t.Helper()
	prog, errs := Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
		"  PLUGGABLEWIDGET 'com.example.W' w1 ( " + prop + " )\n}")
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse %q: %v", prop, e)
		}
		t.FailNow()
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	if len(page.Widgets) == 0 {
		t.Fatalf("no widget parsed from %q", prop)
	}
	return page.Widgets[0].Properties["createFileAction"]
}

// The unambiguous action forms reach the AST as an action.
func TestNamedActionSlot_ActionFormsReachTheAST(t *testing.T) {
	cases := map[string]string{
		"createFileAction: show_page M.Detail":    "showPage",
		"createFileAction: save_changes":          "save",
		"createFileAction: close_page":            "close",
		"createFileAction: open_link 'https://x'": "openLink",
	}
	for prop, wantType := range cases {
		t.Run(prop, func(t *testing.T) {
			got, ok := namedSlotProp(t, prop).(*ast.ActionV3)
			if !ok {
				t.Fatalf("property is %T, want *ast.ActionV3 — the value parses as "+
					"something the executor cannot write into an action slot",
					namedSlotProp(t, prop))
			}
			if got.Type != wantType {
				t.Errorf("action type = %q, want %q", got.Type, wantType)
			}
		})
	}
}

// The overlapping forms still parse as a DATA SOURCE, on purpose: putting the
// action alternative first would read a chart series' `staticDataSource:
// microflow M.X` as an action. This pins that, so a later reordering that looks
// like a simplification fails here rather than silently breaking chart series.
func TestNamedActionSlot_MicroflowStillParsesAsADataSource(t *testing.T) {
	got := namedSlotProp(t, "createFileAction: microflow M.ACT_CreateFile")
	ds, ok := got.(*ast.DataSourceV3)
	if !ok {
		t.Fatalf("property is %T, want *ast.DataSourceV3 — the datasource alternative "+
			"must keep winning this overlap; the executor converts it using the "+
			"widget definition", got)
	}
	if ds.Type != "microflow" || ds.Reference != "M.ACT_CreateFile" {
		t.Errorf("parsed %s %s, want microflow M.ACT_CreateFile", ds.Type, ds.Reference)
	}
}
