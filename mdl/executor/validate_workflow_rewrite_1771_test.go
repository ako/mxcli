// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendix9MultiUserTaskWorkflow is a workflow as a Mendix 9.x–10.11 project
// stores it (ako/mxcli#1771): a multi-user task is a Workflows$UserTask whose
// UserTaskCompletion, a MultiInputCompletion, holds its rule and participants;
// a single-user task's is a SingleInputCompletion.
func mendix9MultiUserTaskWorkflow() map[string]any {
	first := map[string]any{"$ID": "outcome-fast", "$Type": "Workflows$UserTaskOutcome", "Value": "Fast"}
	second := map[string]any{"$ID": "outcome-furious", "$Type": "Workflows$UserTaskOutcome", "Value": "Furious"}
	return map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{
			3,
			map[string]any{"$Type": "Workflows$UserTask", "Name": "userTask1",
				"OnCreatedEvent":     map[string]any{"$Type": "Workflows$NoEvent"},
				"UserTaskCompletion": map[string]any{"$Type": "Workflows$SingleInputCompletion"}},
			map[string]any{"$Type": "Workflows$UserTask", "Name": "userTask2",
				"OnCreatedEvent": map[string]any{"$Type": "Workflows$NoEvent"},
				"Outcomes":       []any{3, first, second},
				"UserTaskCompletion": map[string]any{"$Type": "Workflows$MultiInputCompletion",
					"CompletionCriteria": map[string]any{"$Type": "Workflows$VetoCompletionCriteria", "VetoOutcomePointer": "outcome-furious"},
					"TargetUserInput":    map[string]any{"$Type": "Workflows$AbsoluteAmountUserInput", "Amount": 2}}},
		}},
	}
}

// The rewrite guard refuses a statement that would reset a stored multi-user
// task's rule or participants. Below 10.12 they are stored on the task's
// UserTaskCompletion, where the guard did not look, so a rewrite on 9.24 reset
// both to the defaults silently.
func TestWorkflowRewrite_SeesAMendix9MultiUserTasksSettings(t *testing.T) {
	stmt := parseWorkflowStmt(t, wfRewriteTwoTasks)
	err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, mendix9MultiUserTaskWorkflow()), "wf1", "M.W", stmt)
	if err == nil || !strings.Contains(err.Error(), "veto") {
		t.Fatalf("a rewrite dropping a 9.x multi-user task's veto rule must be refused naming it, got %v", err)
	}

	// Control: restating the rule and the participants loses nothing.
	restated := parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task userTask1 'User Task' page M.P outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P participants 2 decide by veto 'Furious' outcomes 'Fast' { } 'Furious' { };
end workflow;`)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, mendix9MultiUserTaskWorkflow()), "wf1", "M.W", restated); err != nil {
		t.Fatalf("a rewrite that restates the rule and participants must be allowed, got %v", err)
	}
}
