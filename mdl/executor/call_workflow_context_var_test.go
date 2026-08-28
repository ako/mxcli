// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: a CALL WORKFLOW parameter mapping written across two
// lines produced CE0109 "Undefined variable 'Request\n  '". The same mapping on
// one line was fine, and CALL MICROFLOW was fine at any width.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// workflowContextVarFor builds the microflow source and returns the stored workflow
// context variable, going through the real visitor so the whitespace the parser
// preserves is present exactly as it is in an actual run.
func workflowContextVarFor(t *testing.T, body string) string {
	t.Helper()
	src := "create microflow W.MF ( $Request: W.Request )\nbegin\n" + body + "\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", body, errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected a microflow, got %T", prog.Statements[0])
	}
	stmt, ok := mf.Body[0].(*ast.CallWorkflowStmt)
	if !ok {
		t.Fatalf("expected a CallWorkflowStmt, got %T", mf.Body[0])
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	fb.addCallWorkflowAction(stmt)
	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if call, ok := act.Action.(*microflows.WorkflowCallAction); ok {
			return call.WorkflowContextVariable
		}
	}
	t.Fatal("no WorkflowCallAction was built")
	return ""
}

func TestCallWorkflowContextVariableIsNeverWhitespacePadded(t *testing.T) {
	// A workflow's context is stored as a NAME. The argument arrives as an
	// expression source, and the visitor deliberately preserves an expression's
	// trailing whitespace so multi-line expressions round-trip — so the newline
	// rode straight into the name and the build failed CE0109.
	for _, tc := range []struct{ name, body string }{
		{"one line", "  $wf = call workflow W.WF (Context = $Request);"},
		{"mapping on its own line", "  $wf = call workflow W.WF (\n    Context = $Request\n  );"},
		{"positional, multi-line", "  $wf = call workflow W.WF (\n    $Request\n  );"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := workflowContextVarFor(t, tc.body)
			if got != "Request" {
				t.Errorf("context variable = %q, want %q", got, "Request")
			}
			// Stated separately: the failure was invisible in most output because
			// the name LOOKS right until you print it with %q.
			if got != strings.TrimSpace(got) {
				t.Errorf("context variable carries surrounding whitespace: %q", got)
			}
		})
	}
}
