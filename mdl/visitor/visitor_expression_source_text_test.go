// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Four places took an expression's text with ANTLR's GetText(), which joins the
// tokens WITHOUT the whitespace between them. Literals survive (one token each)
// and so do operators like `+`, so `'a' + 'b'` and `$currentObject` looked fine
// — but a keyword operator fuses with its neighbours: `$a and $b` became
// `$aand$b`, `if $x then 'a' else 'b'` became `if$xthen'a'else'b'`. The fused
// text was written into the model as the expression (measured on a copy of
// ako/TestApp: a page action's microflow argument stored "trueandfalse", a REST
// call parameter "if$xthen$idelse'none'"), while check -p --references passed.
//
// Same class as the MDL-comment leak (stripMDLComments): those six microflow
// sites were moved to extractExpressionText, and these four were missed.

func buildExprSrc(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	return prog
}

func TestExpressionSourceText_PageMicroflowArguments(t *testing.T) {
	prog := buildExprSrc(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  actionbutton b (caption: 'Go', action: microflow M.ACT(Flag: $a and $b, Mode: if $x then 'a' else 'b', Obj: $currentObject))
}`)
	btn := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	act, ok := btn.Properties["Action"].(*ast.ActionV3)
	if !ok {
		t.Fatalf("no action on the button: %#v", btn.Properties)
	}
	want := map[string]string{"Flag": "$a and $b", "Mode": "if $x then 'a' else 'b'", "Obj": "$currentObject"}
	for _, a := range act.Args {
		if a.Value != want[a.Name] {
			t.Errorf("argument %s stored %q, want %q", a.Name, a.Value, want[a.Name])
		}
	}
}

func TestExpressionSourceText_ContentParams(t *testing.T) {
	prog := buildExprSrc(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  dynamictext t (content: 'x {1}', contentparams: [{1} = if $x then 'a' else 'b'])
}`)
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	params := w.GetContentParams()
	if len(params) != 1 || params[0].Value != "if $x then 'a' else 'b'" {
		t.Errorf("contentparams stored %#v, want the expression with its spacing", params)
	}
}

func TestExpressionSourceText_SendRestRequestParameters(t *testing.T) {
	prog := buildExprSrc(t, `create microflow M.F ($x: boolean, $a: boolean, $b: boolean) begin
  send rest request M.C.Op with ($Id = if $x then 1 else 2, $Flag = $a and $b);
end;`)
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var got []ast.SendRestParamDef
	for _, s := range mf.Body {
		if r, ok := s.(*ast.SendRestRequestStmt); ok {
			got = r.Parameters
		}
	}
	want := map[string]string{"Id": "if $x then 1 else 2", "Flag": "$a and $b"}
	if len(got) != 2 {
		t.Fatalf("parameters = %#v", got)
	}
	for _, p := range got {
		if p.Expression != want[p.Name] {
			t.Errorf("parameter %s stored %q, want %q", p.Name, p.Expression, want[p.Name])
		}
	}
}

func TestExpressionSourceText_DynamicDatabaseQuery(t *testing.T) {
	prog := buildExprSrc(t, `create microflow M.F ($x: boolean) begin
  $r = execute database query M.C.Q dynamic if $x then 'select 1' else 'select 2';
end;`)
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	for _, s := range mf.Body {
		if q, ok := s.(*ast.ExecuteDatabaseQueryStmt); ok {
			if q.DynamicQuery != "if $x then 'select 1' else 'select 2'" {
				t.Errorf("dynamic query stored %q, want the expression with its spacing", q.DynamicQuery)
			}
			return
		}
	}
	t.Fatal("no execute database query statement")
}

// Control: a single-token value was never affected, and must stay exactly as is.
// And an MDL comment between operands must not end up inside the expression.
func TestExpressionSourceText_ControlsAndComments(t *testing.T) {
	prog := buildExprSrc(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  actionbutton b (caption: 'Go', action: microflow M.ACT(Obj: $currentObject, N: 'a' + 'b', C: $a -- why
    and $b))
}`)
	act := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Properties["Action"].(*ast.ActionV3)
	want := map[string]string{"Obj": "$currentObject", "N": "'a' + 'b'", "C": "$a \n    and $b"}
	for _, a := range act.Args {
		if a.Name == "C" {
			v, _ := a.Value.(string)
			if v == "$aand$b" || v == "" || containsComment(v) {
				t.Errorf("argument C stored %q: fused or carrying the comment", a.Value)
			}
			continue
		}
		if a.Value != want[a.Name] {
			t.Errorf("argument %s stored %q, want %q", a.Name, a.Value, want[a.Name])
		}
	}
}

func containsComment(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '-' && s[i+1] == '-' {
			return true
		}
	}
	return false
}
