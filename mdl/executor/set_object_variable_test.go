// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The repro from mendixlabs/mxcli#1323, verbatim: two Reference retrieves
// followed from the association's FROM entity, so both $Cursor and $Next are
// single G46.Group objects, then `set $Cursor = $Next`. It passed
// `check --references` and `exec`, and mx check answered
// [CE7247] "Variable 'Cursor' does not have a primitive type."
const setObjectRepro = `create module "G46";
create persistent entity "G46"."Group" ("Name": String(50));
create persistent entity "G46"."Node" ("Name": String(50));
create association "G46"."Node_Group" from "G46"."Node" to "G46"."Group" type Reference;
create microflow "G46"."GET_OtherGroup" ($A: "G46"."Node", $B: "G46"."Node") returns "G46"."Group" as $Cursor
begin
  retrieve $Cursor from $A/G46.Node_Group;
  retrieve $Next from $B/G46.Node_Group;
  set $Cursor = $Next;
  return $Cursor;
end;
/
`

// checkScriptMicroflows runs check --references' per-statement validation over
// every microflow in src and returns the joined errors.
func checkScriptMicroflows(t *testing.T, src string) string {
	t.Helper()
	ctx := assocShapeCtx(t)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, st := range prog.Statements {
		if _, ok := st.(*ast.CreateMicroflowStmt); !ok {
			continue
		}
		if err := validateWithContext(ctx, st, sc); err != nil {
			out = append(out, err.Error())
		}
	}
	return strings.Join(out, "\n")
}

// check --references refuses the #1323 repro, naming the CE7247 it prevents.
func TestCheckRefusesSetOnAssociationRetrievedObject(t *testing.T) {
	got := checkScriptMicroflows(t, setObjectRepro)
	if !strings.Contains(got, "CE7247") || !strings.Contains(got, "$Cursor") {
		t.Fatalf("check accepted `set` on object variable $Cursor; got %q", got)
	}
}

// Controls for the check path: the shapes where the retrieve yields a LIST
// keep passing, because `set` on a list is a Change list Replace (ako/mxcli#949).
func TestCheckAcceptsSetOnAssociationRetrievedList(t *testing.T) {
	for name, mf := range map[string]string{
		// Reference followed from its TO entity: the reverse is a list.
		"reverse Reference": `create microflow C88.M ($X: C88.B, $Y: C88.B)
begin
  retrieve $L from $X/C88.R_def;
  retrieve $M from $Y/C88.R_def;
  set $L = $M;
end;
/
`,
		// ReferenceSet from the FROM entity: a list.
		"ReferenceSet": `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  retrieve $L from $X/C88.RS_def;
  retrieve $M from $Y/C88.RS_def;
  set $L = $M;
end;
/
`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := checkScriptMicroflows(t, mf); got != "" {
				t.Errorf("check refused `set` on a list: %s", got)
			}
		})
	}
}

// The same refusal for a stored association, from the project rather than the
// script, and for object variables check knows without one (a parameter).
func TestCheckRefusesSetOnObjectVariable(t *testing.T) {
	for name, mf := range map[string]string{
		"stored Reference forward": `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  retrieve $O from $X/C88.R_def;
  retrieve $P from $Y/C88.R_def;
  set $O = $P;
end;
/
`,
		"object parameter": `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  set $X = $Y;
end;
/
`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := checkScriptMicroflows(t, mf); !strings.Contains(got, "CE7247") {
				t.Errorf("check accepted `set` on an object variable; got %q", got)
			}
		})
	}
}

// exec: the builder refuses `set` on an object variable rather than writing a
// Change variable action mxbuild rejects with CE7247.
func TestSetOnObjectVariableRefusedByBuilder(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Cursor": "G46.Group", "Next": "G46.Group"},
		declaredVars: map[string]string{},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.MfSetStmt{Target: "Cursor", Value: &ast.VariableExpr{Name: "Next"}},
	}, nil)
	if got := strings.Join(fb.errors, "\n"); !strings.Contains(got, "CE7247") || !strings.Contains(got, "$Cursor") {
		t.Fatalf("builder accepted `set` on object variable $Cursor; errors = %q", got)
	}
}
