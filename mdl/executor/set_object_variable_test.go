// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
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
// script.
func TestCheckRefusesSetOnObjectVariable(t *testing.T) {
	mf := `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  retrieve $O from $X/C88.R_def;
  retrieve $P from $Y/C88.R_def;
  set $O = $P;
end;
/
`
	if got := checkScriptMicroflows(t, mf); !strings.Contains(got, "CE7247") {
		t.Errorf("check accepted `set` on an object variable; got %q", got)
	}
}

// An object plain check can see (a parameter) is MDL-SET01's to report; the
// reference validator, which check --references runs as well, stays quiet on
// it so the author sees the refusal once.
func TestCheckReferencesLeavesVisibleObjectsToMDLSET01(t *testing.T) {
	mf := `create microflow C88.M ($X: C88.A, $Y: C88.A)
begin
  set $X = $Y;
end;
/
`
	if got := checkScriptMicroflows(t, mf); got != "" {
		t.Errorf("reference validator reported what MDL-SET01 owns: %q", got)
	}
	if hits := setObjectRuleHits(t, mf); len(hits) != 1 {
		t.Errorf("want one %s, got %q", setObjectRule, hits)
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

// setObjectRuleHits parses src and returns the MDL-SET01 messages plain check
// (ValidateMicroflow / ValidateNanoflow, no project) reports for it.
func setObjectRuleHits(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var hits []string
	for _, st := range prog.Statements {
		var vs []linter.Violation
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			vs = ValidateMicroflow(s)
		case *ast.CreateNanoflowStmt:
			vs = ValidateNanoflow(s)
		}
		for _, v := range vs {
			if v.RuleID == setObjectRule {
				hits = append(hits, v.Message)
			}
		}
	}
	return hits
}

// Plain check, with no project, refuses `set` on a variable it can see is an
// object without one: the object producers that need no association lookup.
func TestPlainCheckRefusesSetOnObjectVariable(t *testing.T) {
	for name, body := range map[string]string{
		"parameter": `set $A = $B;`,
		"create object": `$N = create M.E (Name = 'x');
  set $N = $B;`,
		"retrieve first": `retrieve $F from M.E first;
  set $F = $B;`,
		"loop iterator": `retrieve $L from M.E;
  loop $It in $L begin
    set $It = $B;
  end loop;`,
		"head": `retrieve $L from M.E;
  $H = head($L);
  set $H = $B;`,
	} {
		t.Run(name, func(t *testing.T) {
			src := "create microflow M.MF ($A: M.E, $B: M.E)\nbegin\n  " + body + "\nend;\n"
			hits := setObjectRuleHits(t, src)
			if len(hits) != 1 || !strings.Contains(hits[0], "CE7247") {
				t.Errorf("want one %s naming CE7247, got %q", setObjectRule, hits)
			}
		})
	}
	nf := "create nanoflow M.NF ($A: M.E, $B: M.E)\nbegin\n  set $A = $B;\nend;\n"
	if hits := setObjectRuleHits(t, nf); len(hits) != 1 {
		t.Errorf("nanoflow: want one %s, got %q", setObjectRule, hits)
	}
}

// Controls: what plain check cannot or must not call an object stays accepted
// — a primitive, a list (Change list Replace, ako/mxcli#949), a member path,
// and an association retrieve, whose cardinality needs the project.
func TestPlainCheckAcceptsSetOnNonObject(t *testing.T) {
	for name, body := range map[string]string{
		"primitive": `declare $N Integer = 0;
  set $N = 1;`,
		"list parameter": `set $Ls = $Ls2;`,
		"list retrieve": `retrieve $L from M.E;
  set $L = $Ls;`,
		"member path": `set $A/Name = 'x';`,
		"association retrieve": `retrieve $C from $A/M.E_Other;
  retrieve $D from $B/M.E_Other;
  set $C = $D;`,
	} {
		t.Run(name, func(t *testing.T) {
			src := "create microflow M.MF ($A: M.E, $B: M.E, $Ls: list of M.E, $Ls2: list of M.E)\nbegin\n  " + body + "\nend;\n"
			if hits := setObjectRuleHits(t, src); len(hits) != 0 {
				t.Errorf("refused a set that is not on a known object: %q", hits)
			}
		})
	}
}

// mxbuild words CE7247 differently for a parameter (measured on 11.14.0), and
// the message quotes what the author will see at build time.
func TestPlainCheckQuotesParameterRejection(t *testing.T) {
	hits := setObjectRuleHits(t, "create microflow M.MF ($A: M.E, $B: M.E)\nbegin\n  set $A = $B;\nend;\n")
	if len(hits) != 1 || !strings.Contains(hits[0], `"Parameter 'A' cannot be changed"`) {
		t.Errorf("want the parameter wording of CE7247, got %q", hits)
	}
}
