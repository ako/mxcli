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

// programViolations runs the real ValidateProgram wiring, so a rule that is
// written but never reached by `check` / `exec` fails the test.
func programViolations(t *testing.T, src string) [][2]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out [][2]string
	for _, v := range ValidateProgram(prog, "") {
		out = append(out, [2]string{v.RuleID, v.Message})
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

// MDL-WF03 — every outcome that is not Module.Enumeration.Value is flagged.
//
// This test used to assert that the bare 'Reopened' was ACCEPTED and only the
// free-text 'Confirmed closed' flagged. That was the defect: a bare identifier
// is written verbatim into EnumerationValueConditionOutcome.Value, which the
// Mendix loader refuses, leaving a project Studio Pro and mxbuild cannot open
// (ako/mxcli#1031, ako/mxcli#1065). Both values are now errors — with different
// advice, since one author needs a qualifier and the other needs a real name.
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
	var flagged []string
	for _, v := range vs {
		if v[0] == "MDL-WF03" {
			flagged = append(flagged, v[1])
		}
	}
	if len(flagged) != 2 {
		t.Fatalf("expected MDL-WF03 on BOTH outcomes, got %d: %v", len(flagged), flagged)
	}
	var sawBare, sawFreeText bool
	for _, m := range flagged {
		if strings.Contains(m, "Reopened") {
			sawBare = true
		}
		if strings.Contains(m, "Confirmed closed") {
			sawFreeText = true
		}
	}
	if !sawBare || !sawFreeText {
		t.Errorf("MDL-WF03 must name both outcomes, got %v", flagged)
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

// MDL-WF06 — an enumeration decision whose outcomes omit the empty value.
//
// Measured on mxbuild 11.10.0: the decision below is 1 error, CE6686 ("The
// current outcomes of the decision activity do not match the configured
// expression. Regenerate the outcomes."), and adding `” -> { }` takes the same
// project to 0. See TestValidateWorkflow_EnumDecisionWithEmptyOutcomeClean for
// the control.
func TestValidateWorkflow_EnumDecisionWithoutEmptyOutcome(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Kind'
    outcomes
      'WF.Kind.Standard' -> { }
      'WF.Kind.Priority' -> { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF06") {
		t.Fatalf("expected MDL-WF06 for an enum decision with no empty outcome, got %v", vs)
	}
	for _, v := range vs {
		if v[0] == "MDL-WF06" && !strings.Contains(v[1], "CE6686") {
			t.Errorf("MDL-WF06 should name CE6686, got %q", v[1])
		}
	}
}

// The control: the same decision with the empty outcome is clean. Without this
// the rule could be "always fires on a decision" and the test above would still
// pass.
func TestValidateWorkflow_EnumDecisionWithEmptyOutcomeClean(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Kind'
    outcomes
      'WF.Kind.Standard' -> { }
      'WF.Kind.Priority' -> { }
      '' -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF06") {
		t.Fatalf("an enum decision carrying the empty outcome must not trigger MDL-WF06, got %v", vs)
	}
}

// A boolean decision needs no empty outcome — mxbuild accepts true/false alone
// (0 errors, measured) — so MDL-WF06 must not fire on one.
func TestValidateWorkflow_BooleanDecisionNoWF06(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Total > 1000'
    outcomes
      true -> { }
      false -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF06") {
		t.Fatalf("boolean decision must not trigger MDL-WF06, got %v", vs)
	}
}

// A call-microflow activity branching on an enumeration return needs the empty
// outcome for the same reason: measured 1 error, CE6686 ("The current outcomes
// of the call microflow activity do not match the configured microflow"), and 0
// with `” -> { }`.
func TestValidateWorkflow_CallMicroflowEnumOutcomesWithoutEmpty(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  call microflow WF.ACT as callMicroflow1
    outcomes
      'WF.Kind.Standard' -> { }
      'WF.Kind.Priority' -> { }
  ;
end workflow;`
	vs := workflowViolations(t, src)
	if !hasRule(vs, "MDL-WF06") {
		t.Fatalf("expected MDL-WF06 for enum call-microflow outcomes with no empty outcome, got %v", vs)
	}
	for _, v := range vs {
		if v[0] == "MDL-WF06" && !strings.Contains(v[1], "call microflow") {
			t.Errorf("MDL-WF06 should name the activity kind, got %q", v[1])
		}
	}
}

// A void call-microflow activity carries a single DEFAULT outcome, which is not
// an enumeration branch — MDL-WF06 must leave it alone.
func TestValidateWorkflow_CallMicroflowDefaultOutcomeNoWF06(t *testing.T) {
	src := wfPreamble + `create microflow WF.ACT ($Ctx: WF.Ctx) begin return; end;
create workflow WF.W parameter $Ctx: WF.Ctx
begin
  call microflow WF.ACT as callMicroflow1;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF06") {
		t.Fatalf("a call microflow with no explicit outcomes must not trigger MDL-WF06, got %v", vs)
	}
}

// alterWorkflowViolations parses MDL and runs ValidateAlterWorkflow on every
// ALTER WORKFLOW statement.
func alterWorkflowViolations(t *testing.T, src string) [][2]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out [][2]string
	for _, stmt := range prog.Statements {
		if wf, ok := stmt.(*ast.AlterWorkflowStmt); ok {
			for _, v := range ValidateAlterWorkflow(wf) {
				out = append(out, [2]string{v.RuleID, v.Message})
			}
		}
	}
	return out
}

// MDL-WF06 reaches ALTER as well as CREATE. Measured: inserting this decision
// into a project that was at 0 errors takes `mx check` to 1, CE6686 — and
// nothing looked at an ALTER-introduced activity.
func TestValidateAlterWorkflow_InsertedEnumDecisionWithoutEmptyOutcome(t *testing.T) {
	src := `alter workflow WF.W insert after decision1
  decision decision9 '$WorkflowContext/Kind'
    outcomes
      'WF.Kind.Standard' -> { }
      'WF.Kind.Priority' -> { }
  ;`
	if vs := alterWorkflowViolations(t, src); !hasRule(vs, "MDL-WF06") {
		t.Fatalf("expected MDL-WF06 for an ALTER-inserted enum decision, got %v", vs)
	}
}

// The control: the same insert carrying the empty outcome is clean.
func TestValidateAlterWorkflow_InsertedEnumDecisionWithEmptyOutcomeClean(t *testing.T) {
	src := `alter workflow WF.W insert after decision1
  decision decision9 '$WorkflowContext/Kind'
    outcomes
      'WF.Kind.Standard' -> { }
      'WF.Kind.Priority' -> { }
      '' -> { }
  ;`
	if vs := alterWorkflowViolations(t, src); hasRule(vs, "MDL-WF06") {
		t.Fatalf("an ALTER-inserted enum decision with the empty outcome must be clean, got %v", vs)
	}
}

// An ALTER that only removes or renames introduces no activity and must stay
// silent — the rule reads what the statement adds, not the stored workflow.
func TestValidateAlterWorkflow_NonInsertingOpsNoWF06(t *testing.T) {
	src := `alter workflow WF.W drop activity decision9;
alter workflow WF.W set display 'Approval';`
	if vs := alterWorkflowViolations(t, src); len(vs) > 0 {
		t.Fatalf("non-inserting ALTER ops must produce no violations, got %v", vs)
	}
}

// MDL-WF03 — the three-row control that fixes the rule's threshold.
// EnumerationValueConditionOutcome.Value is parsed by the Mendix LOADER, so a
// value it rejects is not a CE number: the project will not open at all
// (StorageLoadException). Measured on 11.10.0, one workflow per copy of the
// same app: 1 segment and 2 segments both make the project unloadable, 3
// segments checks at 0 errors. See ako/mxcli#1031 and ako/mxcli#1065.
func TestValidateWorkflow_EnumOutcomeMustBeQualified(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		flagged bool
	}{
		{"bare value", "OutcomeA", true},
		{"enum-qualified only", "Status.OutcomeA", true},
		{"fully qualified", "WFP.Status.OutcomeA", false},
		{"four segments", "A.B.C.D", true},
		{"free text", "Confirmed closed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Status'
    outcomes
      '` + tc.value + `' -> { }
      '' -> { }
  ;
end workflow;`
			vs := workflowViolations(t, src)
			if got := hasRule(vs, "MDL-WF03"); got != tc.flagged {
				t.Fatalf("MDL-WF03 fired = %v, want %v for %q (violations: %v)", got, tc.flagged, tc.value, vs)
			}
		})
	}
}

// The empty outcome an enumeration decision must carry ("none of the above",
// which Studio Pro writes on every enum decision) is not an identifier and must
// never be flagged — otherwise the rule refuses the only shape that builds.
func TestValidateWorkflow_EmptyEnumOutcomeAccepted(t *testing.T) {
	src := wfPreamble + `create workflow WF.W parameter $Ctx: WF.Ctx
begin
  decision '$Ctx/Status'
    outcomes
      'WFP.Status.OutcomeA' -> { }
      '' -> { }
  ;
end workflow;`
	if vs := workflowViolations(t, src); hasRule(vs, "MDL-WF03") {
		t.Fatalf("the empty enum outcome must not trigger MDL-WF03, got %v", vs)
	}
}

// A bare value reaches storage through ALTER WORKFLOW … INSERT BRANCH too, so
// the guard covers it. Without this the corrupting write is one keyword away
// from the one that was fixed.
func TestValidateAlterWorkflow_InsertBranchOutcomeMustBeQualified(t *testing.T) {
	bare := `alter workflow WF.W insert condition 'OutcomeA' on 'Decision' { };`
	if vs := programViolations(t, bare); !hasRule(vs, "MDL-WF03") {
		t.Fatalf("expected MDL-WF03 for a bare INSERT CONDITION value, got %v", vs)
	}
	qualified := `alter workflow WF.W insert condition 'WFP.Status.OutcomeA' on 'Decision' { };`
	if vs := programViolations(t, qualified); hasRule(vs, "MDL-WF03") {
		t.Fatalf("a qualified INSERT CONDITION value must be accepted, got %v", vs)
	}
	// The three keyword conditions are Boolean/Void outcomes whatever their
	// casing — the mutator lower-cases before dispatching — so they must not be
	// mistaken for an unqualified enumeration value.
	for _, kw := range []string{"Default", "default", "true", "FALSE"} {
		src := `alter workflow WF.W insert condition '` + kw + `' on 'Decision' { };`
		if vs := programViolations(t, src); hasRule(vs, "MDL-WF03") {
			t.Errorf("INSERT CONDITION %q must not trigger MDL-WF03, got %v", kw, vs)
		}
	}
}
