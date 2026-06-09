// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func TestMapWorkflow(t *testing.T) {
	b := &Backend{}
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Do"}
	call.Name = "callDo"
	call.Outcomes = []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{}}
	start := &workflows.StartWorkflowActivity{}
	start.Name = "start"
	end := &workflows.EndWorkflowActivity{}
	end.Name = "end"

	wf := &workflows.Workflow{
		Name:         "Approve",
		WorkflowName: "Approve Order",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "M.OrderCtx"},
		Flow:         &workflows.Flow{Activities: []workflows.WorkflowActivity{start, call, end}},
	}

	content, err := b.mapWorkflow(wf)
	if err != nil {
		t.Fatalf("mapWorkflow: %v", err)
	}
	if content["name"] != "Approve" || content["title"] != "Approve Order" {
		t.Fatalf("workflow shell: %+v", content)
	}
	param, _ := content["parameter"].(map[string]any)
	if param["$Type"] != "Workflows$Parameter" || param["entity"] != "M.OrderCtx" {
		t.Fatalf("parameter: %+v", param)
	}
	flow, _ := content["flow"].(map[string]any)
	acts, _ := flow["activities"].([]any)
	if len(acts) != 3 {
		t.Fatalf("activities: %+v", acts)
	}
	types := make([]string, 3)
	for i, a := range acts {
		types[i] = a.(map[string]any)["$Type"].(string)
	}
	want := []string{"Workflows$StartWorkflowActivity", "Workflows$CallMicroflowActivity", "Workflows$EndWorkflowActivity"}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("activity[%d] = %s, want %s", i, types[i], want[i])
		}
	}
	// CallMicroflow carries its microflow + the Void outcome.
	cm := acts[1].(map[string]any)
	if cm["microflow"] != "M.ACT_Do" {
		t.Fatalf("call microflow: %+v", cm)
	}
	ocs, _ := cm["outcomes"].([]any)
	if len(ocs) != 1 || ocs[0].(map[string]any)["$Type"] != "Workflows$VoidConditionOutcome" {
		t.Fatalf("outcomes: %+v", cm["outcomes"])
	}
}

func TestMapUserTask(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Process"}
	call.Name = "proc"
	approve := &workflows.UserTaskOutcome{Value: "Approve", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}}
	reject := &workflows.UserTaskOutcome{Value: "Reject"}
	ut := &workflows.UserTask{
		Page:       "M.TaskPage",
		UserSource: &workflows.XPathBasedUserSource{XPath: "[Status = 'Draft']"},
		Outcomes:   []*workflows.UserTaskOutcome{approve, reject},
	}
	ut.Name = "Review"
	ut.Caption = "Review it"

	m, err := mapWorkflowActivity(ut)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(UserTask): %v", err)
	}
	if m["$Type"] != "Workflows$SingleUserTaskActivity" || m["name"] != "Review" {
		t.Fatalf("user task: %+v", m)
	}
	if tp, _ := m["taskPage"].(map[string]any); tp["page"] != "M.TaskPage" {
		t.Fatalf("taskPage: %+v", m["taskPage"])
	}
	tg, _ := m["userTargeting"].(map[string]any)
	if tg["$Type"] != "Workflows$XPathUserTargeting" || tg["xPathConstraint"] != "[Status = 'Draft']" {
		t.Fatalf("userTargeting: %+v", tg)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	o0, _ := ocs[0].(map[string]any)
	if o0["value"] != "Approve" {
		t.Fatalf("outcome[0] value: %+v", o0)
	}
	// The Approve outcome carries a sub-flow with the mapped call-microflow.
	flow, _ := o0["flow"].(map[string]any)
	acts, _ := flow["activities"].([]any)
	if len(acts) != 1 || acts[0].(map[string]any)["$Type"] != "Workflows$CallMicroflowActivity" {
		t.Fatalf("outcome[0] flow: %+v", o0["flow"])
	}
}

func TestMapUserTask_MultiRejected(t *testing.T) {
	ut := &workflows.UserTask{IsMulti: true}
	ut.Name = "multi"
	if _, err := mapWorkflowActivity(ut); err == nil {
		t.Error("multi user task should be rejected for now")
	}
}

func TestMapWorkflowActivity_Unsupported(t *testing.T) {
	// An activity type not yet mapped is rejected, not silently dropped.
	d := &workflows.ExclusiveSplitActivity{}
	d.Name = "decide"
	if _, err := mapWorkflowActivity(d); err == nil {
		t.Error("unmapped workflow activity should be rejected")
	}
}
