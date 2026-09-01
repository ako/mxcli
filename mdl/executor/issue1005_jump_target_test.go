// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1005 — a `jump to` whose target names no activity was
// silently written as a jump to ITSELF, because buildJumpTo named the jump after
// its target. Mendix resolves TargetActivity by name, so the only activity
// carrying the missing name was the jump; the build then failed CE6681 ("not
// possible to jump to end activities or jump-to activities"), naming a different
// fault entirely.

func buildWorkflowFrom(t *testing.T, src string) []workflows.WorkflowActivity {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	wf, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.CreateWorkflowStmt", prog.Statements[0])
	}
	acts := buildWorkflowActivities(wf.Activities)
	deduplicateActivityNames(acts)
	return acts
}

// findJumpAndNames returns the single jump in the tree and every non-jump name.
func findJumpAndNames(acts []workflows.WorkflowActivity) (*workflows.JumpToActivity, map[string]bool) {
	names := map[string]bool{}
	var jump *workflows.JumpToActivity
	var walk func([]workflows.WorkflowActivity)
	walk = func(list []workflows.WorkflowActivity) {
		for _, a := range list {
			switch v := a.(type) {
			case *workflows.JumpToActivity:
				jump = v
			case *workflows.CallMicroflowTask:
				names[v.Name] = true
				for _, o := range v.Outcomes {
					switch oo := o.(type) {
					case *workflows.BooleanConditionOutcome:
						if oo.Flow != nil {
							walk(oo.Flow.Activities)
						}
					case *workflows.VoidConditionOutcome:
						if oo.Flow != nil {
							walk(oo.Flow.Activities)
						}
					}
				}
			case *workflows.UserTask:
				names[v.Name] = true
				for _, o := range v.Outcomes {
					if o.Flow != nil {
						walk(o.Flow.Activities)
					}
				}
			}
		}
	}
	walk(acts)
	return jump, names
}

const jumpBackward = `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.StepA with (P = '$WorkflowContext')
    outcomes DEFAULT -> { };
  call microflow M.StepB with (P = '$WorkflowContext')
    outcomes
      true -> { }
      false -> { jump to StepB comment 'retry'; };
end workflow;`

// The jump appears BEFORE its target. This is the case the report does not
// cover: its trigger table says a jump to a real activity is fine, and that
// holds only for a backward jump. Deduplication renames the SECOND activity it
// meets, so in flow order the jump took StepB and the real StepB became StepB2.
const jumpForward = `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.StepA with (P = '$WorkflowContext')
    outcomes
      true -> { }
      false -> { jump to StepB comment 'skip ahead'; };
  call microflow M.StepB with (P = '$WorkflowContext')
    outcomes DEFAULT -> { };
end workflow;`

func TestJumpTo_NeverTakesTheTargetsName(t *testing.T) {
	for name, src := range map[string]string{"backward": jumpBackward, "forward": jumpForward} {
		t.Run(name, func(t *testing.T) {
			jump, names := findJumpAndNames(buildWorkflowFrom(t, src))
			if jump == nil {
				t.Fatal("no jump activity was built")
			}
			if jump.Name == jump.TargetActivity {
				t.Errorf("jump is named after its target (%q) — it targets itself", jump.Name)
			}
			if !names[jump.TargetActivity] {
				t.Errorf("target %q is not the name of any activity; names present: %v",
					jump.TargetActivity, names)
			}
			if jump.TargetActivity != "StepB" {
				t.Errorf("TargetActivity = %q, want StepB", jump.TargetActivity)
			}
		})
	}
}

// The caption a user wrote must survive the renaming — it is the only thing
// distinguishing two jumps in Studio Pro.
func TestJumpTo_KeepsTheAuthoredCaption(t *testing.T) {
	jump, _ := findJumpAndNames(buildWorkflowFrom(t, jumpBackward))
	if jump.Caption != "retry" {
		t.Errorf("Caption = %q, want %q", jump.Caption, "retry")
	}
}

// Two jumps in one workflow must still get distinct names.
func TestJumpTo_TwoJumpsGetDistinctNames(t *testing.T) {
	src := `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.StepA with (P = '$WorkflowContext')
    outcomes
      true -> { jump to StepA comment 'again'; }
      false -> { jump to StepA comment 'and again'; };
end workflow;`
	acts := buildWorkflowFrom(t, src)
	seen := map[string]int{}
	var walk func([]workflows.WorkflowActivity)
	walk = func(list []workflows.WorkflowActivity) {
		for _, a := range list {
			switch v := a.(type) {
			case *workflows.JumpToActivity:
				seen[v.Name]++
			case *workflows.CallMicroflowTask:
				seen[v.Name]++
				for _, o := range v.Outcomes {
					if b, ok := o.(*workflows.BooleanConditionOutcome); ok && b.Flow != nil {
						walk(b.Flow.Activities)
					}
				}
			}
		}
	}
	walk(acts)
	for name, n := range seen {
		if n > 1 {
			t.Errorf("name %q used %d times — CE0495", name, n)
		}
	}
}

func workflowRuleIDs(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []string
	for _, stmt := range prog.Statements {
		if wf, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			for _, v := range ValidateWorkflow(wf) {
				out = append(out, v.RuleID)
			}
		}
	}
	return out
}

// MDL-WF05 — the check that did not exist. A jump target is the only
// intra-document reference a workflow has, and nothing resolved it.
func TestMDLWF05_FlagsADanglingJumpTarget(t *testing.T) {
	src := `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.SUB_CheckPackageAvailability with (P = '$WorkflowContext')
    outcomes
      true -> { }
      false -> { jump to callMicroflow2 comment 'back to StepB'; };
end workflow;`
	ids := workflowRuleIDs(t, src)
	found := false
	for _, id := range ids {
		if id == "MDL-WF05" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no MDL-WF05 for an unresolved jump target; got %v", ids)
	}

	// The valid names are not obvious from the script — a `call microflow
	// M.SUB_CheckPackageAvailability` activity is named SUB_CheckPackageAvailability,
	// which is most of why the wrong name is easy to reach — so the rule must
	// list them.
	prog, _ := visitor.Build(src)
	wf := prog.Statements[0].(*ast.CreateWorkflowStmt)
	vs := ValidateWorkflowJumpTargets(wf)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1", len(vs))
	}
	if !strings.Contains(vs[0].Suggestion, "SUB_CheckPackageAvailability") {
		t.Errorf("suggestion does not list the valid target: %q", vs[0].Suggestion)
	}
	if !strings.Contains(vs[0].Message, "CE6681") {
		t.Errorf("message does not name the build error it prevents: %q", vs[0].Message)
	}
}

// The control: a jump to a real activity must NOT be flagged, in both
// directions. Without this the rule could pass by refusing every jump.
func TestMDLWF05_AcceptsAValidJumpTarget(t *testing.T) {
	for name, src := range map[string]string{"backward": jumpBackward, "forward": jumpForward} {
		t.Run(name, func(t *testing.T) {
			for _, id := range workflowRuleIDs(t, src) {
				if id == "MDL-WF05" {
					t.Errorf("MDL-WF05 fired on a valid %s jump", name)
				}
			}
		})
	}
}

// A target inside a nested outcome flow is still a valid target — the collector
// has to recurse, or every jump into a branch would be refused.
func TestMDLWF05_FindsATargetInsideANestedFlow(t *testing.T) {
	src := `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.Outer with (P = '$WorkflowContext')
    outcomes
      true -> {
        call microflow M.InnerStep with (P = '$WorkflowContext')
          outcomes DEFAULT -> { };
      }
      false -> { jump to InnerStep comment 'into the branch'; };
end workflow;`
	for _, id := range workflowRuleIDs(t, src) {
		if id == "MDL-WF05" {
			t.Errorf("MDL-WF05 fired on a jump to an activity nested in an outcome flow")
		}
	}
}

// A jump may not target another jump (CE6681), so a jump's own name must not be
// offered as a valid target.
func TestMDLWF05_AJumpIsNotAValidTarget(t *testing.T) {
	src := `create workflow M.WF
  parameter $WorkflowContext: M.E
begin
  call microflow M.StepA with (P = '$WorkflowContext')
    outcomes
      true -> { jump to StepA comment 'one'; }
      false -> { jump to JumpTo comment 'two'; };
end workflow;`
	found := false
	for _, id := range workflowRuleIDs(t, src) {
		if id == "MDL-WF05" {
			found = true
		}
	}
	if !found {
		t.Error("a jump targeting another jump was accepted — Mendix rejects it with CE6681")
	}
}
