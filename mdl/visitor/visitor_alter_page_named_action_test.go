// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestAlterPage_SetNamedActionSlot_Parses is the grammar half of
// mendixlabs/mxcli#995.
//
// #956 gave a pluggable widget's named action slots a spelling on CREATE PAGE
// (`createFileAction: microflow M.F`), but `alterPageAssignment` fell through to
// propertyValueV3 for any name other than `Action`, and that rule has no
// `microflow <name>` form. The reported symptom, verbatim:
//
//	line 2:37 extraneous input 'MyModule' expecting {DROP, ADD, SET, INSERT, REPLACE, '}'}
//
// so retargeting one slot meant REPLACEing the whole widget and restating every
// other property. Both the quoted (pluggable-property convention) and bare
// spellings from the report must reach the executor as an action, not a scalar.
func TestAlterPage_SetNamedActionSlot_Parses(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		key      string
		wantType string
		target   string
	}{
		{"quoted microflow",
			"alter page MyModule.UploadPage {\n  set 'createFileAction' = microflow MyModule.ACT_CreateFile on fileUploader1;\n};",
			"createFileAction", "microflow", "MyModule.ACT_CreateFile"},
		{"bare microflow",
			"alter page MyModule.UploadPage {\n  set createFileAction = microflow MyModule.ACT_CreateFile on fileUploader1;\n};",
			"createFileAction", "microflow", "MyModule.ACT_CreateFile"},
		{"quoted nanoflow",
			"alter page M.P { set 'onUploadSuccessFile' = nanoflow M.NF_Done on fileUploader1; };",
			"onUploadSuccessFile", "nanoflow", "M.NF_Done"},
		{"quoted show_page",
			"alter page M.P { set 'onSelectionChange' = show_page M.Detail on dg; };",
			"onSelectionChange", "showPage", "M.Detail"},
		{"inside a parenthesised list",
			"alter page M.P { set ('createFileAction' = microflow M.A, 'createImageAction' = microflow M.B) on fileUploader1; };",
			"createImageAction", "microflow", "M.B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("Build: %v", errs)
			}
			op, ok := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
			if !ok {
				t.Fatalf("op type = %T, want *ast.SetPropertyOp", prog.Statements[0].(*ast.AlterPageStmt).Operations[0])
			}
			raw, present := op.Properties[tt.key]
			if !present {
				t.Fatalf("no %q property; got %v", tt.key, op.Properties)
			}
			action, ok := raw.(*ast.ActionV3)
			if !ok {
				t.Fatalf("%s value type = %T, want *ast.ActionV3", tt.key, raw)
			}
			if action.Type != tt.wantType || action.Target != tt.target {
				t.Errorf("action = %s %s, want %s %s", action.Type, action.Target, tt.wantType, tt.target)
			}
		})
	}
}

// TestAlterPage_SetNamedActionSlot_ScalarsStayScalars is the control: adding an
// action alternative must not capture the scalar values the generic assignment
// already carried.
func TestAlterPage_SetNamedActionSlot_ScalarsStayScalars(t *testing.T) {
	tests := []struct {
		src  string
		key  string
		want any
	}{
		{"alter page M.P { set 'showLabel' = false on w; };", "showLabel", false},
		{"alter page M.P { set 'pageSize' = 10 on w; };", "pageSize", 10},
		{"alter page M.P { set Caption = 'Save' on w; };", "Caption", "Save"},
		{"alter page M.P { set 'mode' = 'files' on w; };", "mode", "files"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			prog, errs := Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("Build: %v", errs)
			}
			op := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
			if got := op.Properties[tt.key]; got != tt.want {
				t.Errorf("%s = %#v (%T), want %#v", tt.key, got, got, tt.want)
			}
		})
	}
}
