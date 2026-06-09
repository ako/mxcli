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

func TestMapDecision(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Process"}
	call.Name = "proc"
	dec := &workflows.ExclusiveSplitActivity{
		Expression: "$ctx/Total > 1000",
		Outcomes: []workflows.ConditionOutcome{
			&workflows.BooleanConditionOutcome{Value: true, Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}},
			&workflows.BooleanConditionOutcome{Value: false},
		},
	}
	dec.Name = "decide"
	m, err := mapWorkflowActivity(dec)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(Decision): %v", err)
	}
	if m["$Type"] != "Workflows$ExclusiveSplitActivity" || m["expression"] != "$ctx/Total > 1000" {
		t.Fatalf("decision: %+v", m)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	o0, _ := ocs[0].(map[string]any)
	if o0["$Type"] != "Workflows$BooleanConditionOutcome" || o0["value"] != true {
		t.Fatalf("outcome[0]: %+v", o0)
	}
	if _, ok := o0["flow"]; !ok {
		t.Fatalf("outcome[0] missing sub-flow: %+v", o0)
	}
}

func TestMapParallelSplit(t *testing.T) {
	mk := func(mf string) *workflows.Flow {
		c := &workflows.CallMicroflowTask{Microflow: mf}
		c.Name = "c"
		return &workflows.Flow{Activities: []workflows.WorkflowActivity{c}}
	}
	ps := &workflows.ParallelSplitActivity{Outcomes: []*workflows.ParallelSplitOutcome{
		{Flow: mk("M.A")}, {Flow: mk("M.B")},
	}}
	ps.Name = "split"
	m, err := mapWorkflowActivity(ps)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(ParallelSplit): %v", err)
	}
	if m["$Type"] != "Workflows$ParallelSplitActivity" {
		t.Fatalf("parallel split: %+v", m)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	for i, want := range []string{"M.A", "M.B"} {
		oc := ocs[i].(map[string]any)
		if oc["$Type"] != "Workflows$ParallelSplitOutcome" {
			t.Fatalf("outcome[%d]: %+v", i, oc)
		}
		flow := oc["flow"].(map[string]any)
		act := flow["activities"].([]any)[0].(map[string]any)
		if act["microflow"] != want {
			t.Fatalf("path[%d] microflow = %v, want %s", i, act["microflow"], want)
		}
	}
}

func TestMapJumpTo(t *testing.T) {
	j := &workflows.JumpToActivity{TargetActivity: "ReviewStep"}
	j.Name = "jump"
	m, err := mapWorkflowActivity(j)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(JumpTo): %v", err)
	}
	if m["$Type"] != "Workflows$JumpToActivity" || m["targetActivity"] != "ReviewStep" {
		t.Fatalf("jump to: %+v", m)
	}
}

func TestMapWaitForTimer(t *testing.T) {
	w := &workflows.WaitForTimerActivity{DelayExpression: "addHours([%CurrentDateTime%], 1)"}
	w.Name = "wait"
	m, err := mapWorkflowActivity(w)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(WaitForTimer): %v", err)
	}
	if m["$Type"] != "Workflows$WaitForTimerActivity" || m["delay"] != "addHours([%CurrentDateTime%], 1)" {
		t.Fatalf("wait for timer: %+v", m)
	}
}

func TestMapWorkflowActivity_Unsupported(t *testing.T) {
	// An activity type not yet mapped is rejected, not silently dropped.
	w := &workflows.WaitForNotificationActivity{}
	w.Name = "wait"
	if _, err := mapWorkflowActivity(w); err == nil {
		t.Error("unmapped workflow activity should be rejected")
	}
}

func TestWorkflowMutator_SetProperties(t *testing.T) {
	m := &mcpWorkflowMutator{backend: &Backend{}, moduleName: "M", workflowName: "WF"}
	if err := m.SetProperty("display", "New Name"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetProperty("description", "New Desc"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPropertyWithEntity("parameter", "$ctx", "M.NewCtx"); err != nil {
		t.Fatal(err)
	}
	// Expect ops: /workflowName/text, /title, /workflowDescription/text, /parameter/entity.
	got := map[string]any{}
	for _, op := range m.ops {
		got[op.Path] = op.Operation.Value
	}
	want := map[string]any{
		"/workflowName/text":        "New Name",
		"/title":                    "New Name",
		"/workflowDescription/text": "New Desc",
		"/parameter/entity":         "M.NewCtx",
	}
	for path, v := range want {
		if got[path] != v {
			t.Errorf("op %s = %v, want %v", path, got[path], v)
		}
	}
	// Unsupported workflow-level property and activity ops are rejected.
	if err := m.SetProperty("export_level", "api"); err == nil {
		t.Error("SET export_level should be rejected")
	}
	// SetActivityProperty is still stubbed (no client call), unlike Insert/Drop/
	// Replace which now resolve an activity index live.
	if err := m.SetActivityProperty("x", 0, "page", "p"); err == nil {
		t.Error("SetActivityProperty should be rejected (not yet supported)")
	}
}
