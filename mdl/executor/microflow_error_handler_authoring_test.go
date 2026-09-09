// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1078, second half. Fixing the describer alone made it emit an
// `on error { … }` block on eight statements whose grammar had no onErrorClause
// — MDL that does not parse. Trading a silent drop for a broken script is not a
// fix, so those eight statements now accept the clause.
//
// The reporter's activity is the first row: a create-variable with "custom with
// rollback", which is ErrorHandlingType "Custom" in the metamodel.

// buildFlowFromMDL parses a microflow and returns the objects the flow builder
// produced. Parsed rather than hand-built: the point is that the CLAUSE reaches
// the model from real source text, so a hand-made AST would test nothing.
func buildFlowFromMDL(t *testing.T, body string) *flowBuilder {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb.buildFlowGraph(mf.Body, nil)
	return fb
}

// actionErrorHandlingTypes returns every action activity's ErrorHandlingType.
// All of them, not the first: a handler body contributes activities too, and the
// `set` case needs a declare in front of it.
func actionErrorHandlingTypes(fb *flowBuilder) []microflows.ErrorHandlingType {
	var out []microflows.ErrorHandlingType
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.ActionActivity); ok {
			out = append(out, actionErrorHandlingField(a.Action))
		}
	}
	return out
}

// countErrorHandling returns how many action activities carry the given type.
func countErrorHandling(fb *flowBuilder, want microflows.ErrorHandlingType) int {
	var n int
	for _, t := range actionErrorHandlingTypes(fb) {
		if t == want {
			n++
		}
	}
	return n
}

// Each of the eight statements #1078 opened up, with the reporter's own shape
// first. Before the grammar change every one of these was a parse error.
func TestAuthorOnError_EightStatementsThatCouldNotCarryTheClause(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"declare", "declare $name String = 'NameValue' on error { rollback $x; };"},
		{"set", "declare $name String = 'v';\n$name = 'w' on error { rollback $x; };"},
		{"change object", "change $Car (Brand = 'Opel') on error { rollback $x; };"},
		{"log", "log info node 'B' 'hi' on error { rollback $x; };"},
		{"show page", "show page M.Home on error { rollback $x; };"},
		{"close page", "close page on error { rollback $x; };"},
		{"show message", "show message 'hi' on error { rollback $x; };"},
		{"validation feedback", "validation feedback $Car/Brand message 'bad' on error { rollback $x; };"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, tc.body)

			if len(actionErrorHandlingTypes(fb)) == 0 {
				t.Fatal("the builder produced no action activity")
			}
			// Custom is what Studio Pro's "custom with rollback" stores, and it is
			// what hasCustomErrorHandler needs to see for DESCRIBE to walk the
			// branch back out again. Exactly one activity carries it: the one the
			// clause was written on.
			if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustom); n != 1 {
				t.Errorf("%d activities carry ErrorHandlingType %q, want 1 — DESCRIBE "+
					"gates the whole branch on this value; got %v",
					n, microflows.ErrorHandlingTypeCustom, actionErrorHandlingTypes(fb))
			}

			// The handler's own activities and the error flow must exist, or the
			// clause parsed into nothing.
			var errFlows int
			for _, f := range fb.flows {
				if f.IsErrorHandler {
					errFlows++
				}
			}
			if errFlows != 1 {
				t.Errorf("got %d error-handler flows, want 1 — the handler body was not wired", errFlows)
			}
		})
	}
}

// Control for the table above. Without the clause the action must carry NO
// error-handling type: writing one would make DESCRIBE render `on error` on an
// activity the author never put one on, which is #840 in reverse.
func TestAuthorOnError_AbsentClauseLeavesTheActionUnset(t *testing.T) {
	for _, body := range []string{
		"declare $name String = 'NameValue';",
		"log info node 'B' 'hi';",
		"close page;",
	} {
		fb := buildFlowFromMDL(t, body)
		types := actionErrorHandlingTypes(fb)
		if len(types) == 0 {
			t.Fatalf("%s: no action activity", body)
		}
		for _, errType := range types {
			if errType != "" {
				t.Errorf("%s: ErrorHandlingType = %q, want empty — no clause was written",
					body, errType)
			}
		}
		for _, f := range fb.flows {
			if f.IsErrorHandler {
				t.Errorf("%s: an error-handler flow was created with no clause", body)
			}
		}
	}
}

// `on error without rollback` must reach the model as its own value, not collapse
// into Custom — the two differ at runtime.
func TestAuthorOnError_WithoutRollbackIsDistinct(t *testing.T) {
	fb := buildFlowFromMDL(t,
		"declare $name String = 'v' on error without rollback { rollback $x; };")
	if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustomWithoutRollback); n != 1 {
		t.Errorf("%d activities carry %q, want 1; got %v", n,
			microflows.ErrorHandlingTypeCustomWithoutRollback, actionErrorHandlingTypes(fb))
	}
}

// MDL077: `set` is one statement form spanning activities that can and cannot
// hold the clause. Change variable can; list operation and aggregate have no
// ErrorHandlingType in the metamodel at all, so the clause is refused rather than
// accepted and discarded.
func TestMDL077_RefusesOnErrorWhereTheActivityHasNoField(t *testing.T) {
	refused := []struct{ name, body string }{
		{"list operation", "$h = head($list) on error { rollback $x; };"},
		{"aggregate", "$n = count($list) on error { rollback $x; };"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			out := checkMicroflowBodyForTest(t, tc.body)
			if !strings.Contains(out, "MDL077") {
				t.Errorf("expected MDL077 for %s, got:\n%s", tc.name, out)
			}
		})
	}

	// Control: the plain form of the SAME statement keyword is accepted, so the
	// rule is discriminating between activities and not just banning `set`.
	if out := checkMicroflowBodyForTest(t,
		"declare $a String = 'v';\n$a = 'w' on error { rollback $x; };"); strings.Contains(out, "MDL077") {
		t.Errorf("MDL077 fired on a plain set, which IS a Change variable activity:\n%s", out)
	}
}

// checkMicroflowBodyForTest runs the microflow validator over a body and returns
// the rule IDs it reported.
func checkMicroflowBodyForTest(t *testing.T, body string) string {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	v := &microflowValidator{
		mfName:        "M.ACT_T",
		emptyListVars: map[string]bool{},
		varKinds:      map[string]exprcheck.TypeKind{},
	}
	v.walkBody(mf.Body)
	var b strings.Builder
	for _, viol := range v.violations {
		b.WriteString(viol.RuleID + ": " + viol.Message + "\n")
	}
	return b.String()
}
