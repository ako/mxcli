// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Studio Pro's "Visible: based on attribute value" — stored as a
// ConditionalVisibilitySettings with an Attribute and one Enumerations$Condition
// per value — had no MDL spelling, so describe → exec dropped it and the widget
// became always visible (Administration.Account_Edit: 8 conditions → 0, with
// mx check clean). `Visible: Attr in (values)` names the values that show it.
func TestVisibleWhenAttributeIn(t *testing.T) {
	for _, tc := range []struct {
		src    string
		attr   string
		values []string
	}{
		{"textbox tb (Attribute: Name, Visible: IsLocalUser in (false))", "IsLocalUser", []string{"false"}},
		{"container c (Visible: Status in (Running, empty)) { dynamictext t (Content: 'x') }", "Status", []string{"Running", "empty"}},
		{"container c (Visible: \"Type\" in (Completed)) { dynamictext t (Content: 'x') }", "Type", []string{"Completed"}},
	} {
		prog, errs := Build("create page M.P (Title: 'x', Layout: A.L) { " + tc.src + " };")
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs)
		}
		w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
		vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3)
		if !ok {
			t.Fatalf("%s: VisibleWhen = %T (%v)", tc.src, w.Properties["VisibleWhen"], w.Properties)
		}
		if vw.Attribute != tc.attr || !reflect.DeepEqual(vw.Values, tc.values) {
			t.Errorf("%s: got %s in %v, want %s in %v", tc.src, vw.Attribute, vw.Values, tc.attr, tc.values)
		}
	}
}

// The existing forms are unchanged.
func TestVisibleExpressionFormsUnchanged(t *testing.T) {
	prog, errs := Build("create page M.P (Title: 'x', Layout: A.L) { textbox tb (Attribute: Name, Visible: [IsActive = true]) };")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	if _, ok := w.Properties["VisibleIf"]; !ok {
		t.Errorf("bracketed Visible no longer lands in VisibleIf: %v", w.Properties)
	}
	if _, ok := w.Properties["VisibleWhen"]; ok {
		t.Errorf("bracketed Visible parsed as an attribute condition")
	}
}
