// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestAlterPage_SetAction_Parses covers the grammar half of the fix.
//
// `alterPageAssignment` special-cased DataSource, Visible and Editable and then
// fell through to `propertyValueV3`, which has no `microflow <name>` form — so
// `SET Action = microflow M.F ON btn` did not parse at all. Retargeting a button
// meant REPLACEing the whole widget, which silently drops any property the
// author did not restate.
//
// The rule now reuses actionExprV3, the same action grammar CREATE PAGE uses, so
// every form is available rather than a subset that has to be extended one bug
// report at a time.
func TestAlterPage_SetAction_Parses(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		verify func(t *testing.T, a *ast.ActionV3)
	}{
		{
			name: "microflow",
			src:  "alter page M.P { set Action = microflow M.ACT_Other on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type != "microflow" {
					t.Errorf("Type = %q, want microflow", a.Type)
				}
				if a.Target != "M.ACT_Other" {
					t.Errorf("Target = %q, want M.ACT_Other", a.Target)
				}
			},
		},
		{
			name: "nanoflow",
			src:  "alter page M.P { set Action = nanoflow M.NF_Other on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type != "nanoflow" {
					t.Errorf("Type = %q, want nanoflow", a.Type)
				}
			},
		},
		{
			name: "save changes with close",
			src:  "alter page M.P { set Action = SAVE_CHANGES CLOSE_PAGE on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type != "save" {
					t.Errorf("Type = %q, want save", a.Type)
				}
				if !a.ClosePage {
					t.Error("ClosePage = false, want true — the close is a flag on the action")
				}
			},
		},
		{
			name: "save changes without close",
			src:  "alter page M.P { set Action = SAVE_CHANGES on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type != "save" || a.ClosePage {
					t.Errorf("Type = %q ClosePage = %v, want save/false", a.Type, a.ClosePage)
				}
			},
		},
		{
			name: "show page",
			src:  "alter page M.P { set Action = SHOW_PAGE M.Other on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type != "showPage" {
					t.Errorf("Type = %q, want showPage", a.Type)
				}
				if a.Target != "M.Other" {
					t.Errorf("Target = %q, want M.Other", a.Target)
				}
			},
		},
		{
			name: "open link",
			src:  "alter page M.P { set Action = OPEN_LINK 'https://example.com' on btnGo; };",
			verify: func(t *testing.T, a *ast.ActionV3) {
				if a.Type == "" {
					t.Error("Type is empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("Build(%q): %v", tt.src, errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("statements = %d, want 1", len(prog.Statements))
			}
			stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
			if !ok {
				t.Fatalf("type = %T, want *ast.AlterPageStmt", prog.Statements[0])
			}
			if len(stmt.Operations) != 1 {
				t.Fatalf("operations = %d, want 1", len(stmt.Operations))
			}
			op, ok := stmt.Operations[0].(*ast.SetPropertyOp)
			if !ok {
				t.Fatalf("op type = %T, want *ast.SetPropertyOp", stmt.Operations[0])
			}
			if op.Target.Widget != "btnGo" {
				t.Errorf("target widget = %q, want btnGo", op.Target.Widget)
			}
			raw, present := op.Properties["Action"]
			if !present {
				t.Fatalf("no Action property; got %v", op.Properties)
			}
			action, ok := raw.(*ast.ActionV3)
			if !ok {
				t.Fatalf("Action value type = %T, want *ast.ActionV3 — a scalar here would be "+
					"written as a plain property and produce an unopenable document", raw)
			}
			tt.verify(t, action)
		})
	}
}

// TestAlterPage_SetAction_DoesNotShadowOtherProperties makes sure adding the
// ACTION alternative did not capture assignments that should still go through
// the generic identifier path — `ActionName` is not `Action`.
func TestAlterPage_SetAction_DoesNotShadowOtherProperties(t *testing.T) {
	prog, errs := Build("alter page M.P { set Caption = 'Save' on btnGo; };")
	if len(errs) > 0 {
		t.Fatalf("Build: %v", errs)
	}
	op := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
	if got, ok := op.Properties["Caption"]; !ok || got != "Save" {
		t.Errorf("Caption = %v (present=%v), want Save", got, ok)
	}
}
