// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// workflowViolations parses MDL, runs ValidateWorkflow on every workflow
// statement, and returns (ruleID, message) pairs.
func workflowViolations(t *testing.T, src string) [][2]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out [][2]string
	for _, stmt := range prog.Statements {
		if wf, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			for _, v := range ValidateWorkflow(wf) {
				out = append(out, [2]string{v.RuleID, v.Message})
			}
		}
	}
	return out
}

func hasRule(vs [][2]string, ruleID string) bool {
	for _, v := range vs {
		if v[0] == ruleID {
			return true
		}
	}
	return false
}

const wfPreamble = `create persistent entity WF.Ctx ( Total: decimal );
create page WF.TaskPage ( title: 'T', layout: Atlas_Core.PopupLayout ) { };
`

// MDL-WF01 — a user task without a page is flagged.
func TestValidateWorkflow_UserTaskWithoutPage(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    outcomes 'Done' { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF01") {
		t.Fatalf("expected MDL-WF01 for user task without a page, got %v", vs)
	}
}

// A user task WITH a page does not trigger MDL-WF01.
func TestValidateWorkflow_UserTaskWithPageClean(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes 'Done' { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF01") {
		t.Fatalf("user task with a page should not trigger MDL-WF01, got %v", vs)
	}
}

// MDL-WF02 — a single outcome carrying nested activities is flagged.
func TestValidateWorkflow_SingleOutcomeWithActivities(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes 'Done' { call microflow WF.ACT; }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF02") {
		t.Fatalf("expected MDL-WF02 for single outcome with activities, got %v", vs)
	}
}

// Two outcomes, one with activities, is allowed (not a single-outcome task).
func TestValidateWorkflow_MultipleOutcomesClean(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  user task ReviewOrder 'Review'
    page WF.TaskPage
    outcomes
      'Approve' { call microflow WF.ACT; }
      'Reject' { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF02") {
		t.Fatalf("multi-outcome task should not trigger MDL-WF02, got %v", vs)
	}
}

// MDL-WF03 — a decision outcome that is not a valid identifier (has a space) is flagged.
func TestValidateWorkflow_FreeTextDecisionOutcome(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      'Reopened' -> { }
      'Confirmed closed' -> { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF03") {
		t.Fatalf("expected MDL-WF03 for free-text decision outcome, got %v", vs)
	}
	// 'Reopened' is a valid identifier and must NOT be flagged; only 'Confirmed closed'.
	var wf03 int
	for _, v := range vs {
		if v[0] == "MDL-WF03" {
			wf03++
			if !strings.Contains(v[1], "Confirmed closed") {
				t.Errorf("MDL-WF03 should name 'Confirmed closed', got %q", v[1])
			}
		}
	}
	if wf03 != 1 {
		t.Fatalf("expected exactly one MDL-WF03 (only 'Confirmed closed'), got %d in %v", wf03, vs)
	}
}

// A boolean decision (true/false) is clean.
func TestValidateWorkflow_BooleanDecisionClean(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      true -> { }
      false -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF03") {
		t.Fatalf("boolean decision should not trigger MDL-WF03, got %v", vs)
	}
}

// MDL-WF04 — a standalone `annotation` in a workflow body is refused.
//
// mxcli placed the Annotation in the workflow's activity flow, where Mendix
// constructs every child with a Flow parent. The result is a project Mendix
// cannot LOAD ("Type ...Workflows.Model.Annotation does not contain a
// constructor with a parameter of type ...Flow") — Studio Pro will not open it
// and `mx check` dies before validating anything. Until the correct container
// is known, refusing beats emitting an unopenable model. (issuetracker #15)
func TestValidateWorkflow_StandaloneAnnotationRefused(t *testing.T) {
	src := wfPreamble + `create workflow WF.Flow
  parameter $Context: WF.Ctx
begin
  annotation 'Escalation path per policy 4.2';
  user task T 'Do it' page WF.TaskPage outcomes 'Approve' { } 'Reject' { };
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF04") {
		t.Fatalf("expected MDL-WF04 for a standalone workflow annotation, got %v", vs)
	}
}

// A workflow without a standalone annotation must not trip MDL-WF04.
func TestValidateWorkflow_NoAnnotationNoWF04(t *testing.T) {
	src := wfPreamble + `create workflow WF.Flow
  parameter $Context: WF.Ctx
begin
  user task T 'Do it' page WF.TaskPage outcomes 'Approve' { } 'Reject' { };
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF04") {
		t.Errorf("MDL-WF04 must not fire without an annotation: %v", vs)
	}
}

// MDL-WF03 must accept the qualified form Studio Pro actually stores. Every
// EnumerationValueConditionOutcome in the demo corpus stores Module.Enum.Value
// (7 of 7 non-empty), so `describe workflow` emits it and the rule refused its
// own output. See ako/mxcli#408.
func TestValidateWorkflow_QualifiedEnumOutcomeAccepted(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      'FactoryManagement.ENUM_InvestigationType.Engineering' -> { }
      'FactoryManagement.ENUM_InvestigationType.Operations' -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF03") {
		t.Fatalf("qualified enum outcome must not trigger MDL-WF03, got %v", vs)
	}
}

// The control for the case above: free text with a space is still refused, so
// widening the rule to accept dots did not turn it off.
func TestValidateWorkflow_QualifiedEnumOutcomeStillRejectsFreeText(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      'Factory Management.ENUM_Kind.A' -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); !hasRule(vs, "MDL-WF03") {
		t.Fatalf("dotted free text with a space must still trigger MDL-WF03, got %v", vs)
	}
}

// MDL-WF05 — a jump may target a named decision or parallel split. Mendix
// resolves JumpToActivity.TargetActivity by activity NAME, and Studio Pro names
// them decision1 / split1 regardless of caption, so MDL needs a name slot on
// every jumpable activity or a described workflow cannot be re-executed.
func TestValidateWorkflow_JumpToNamedDecisionAndSplit(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision decision1 '$Ctx/Total > 1000'
    outcomes
      true -> { }
      false -> { }
  ;
  parallel split split1
    path 1 { jump to decision1; }
    path 2 { }
  ;
  wait for timer timer1 'PT1H';
  wait for notification waitForNotification1;
  jump to split1;
end workflow;`
	vs := workflowViolations(t, src)
	if hasRule(vs, "MDL-WF05") {
		t.Fatalf("jump to a named decision/split must resolve, got %v", vs)
	}
}
