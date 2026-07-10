// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// TestCreateWorkflow_RoundTrip creates a workflow with a user task (one outcome)
// and a microflow task in the flow, then confirms it round-trips through
// ListWorkflows (name, parameter entity, and the activity tree).
func TestCreateWorkflow_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	userTask := &workflows.UserTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Review", Caption: "Review"},
		Page:                 "MyFirstModule.ReviewPage",
		Outcomes:             []*workflows.UserTaskOutcome{{Value: "Approved"}},
	}
	mfTask := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Notify", Caption: "Notify"},
		Microflow:            "MyFirstModule.ACT_Notify",
	}
	wf := &workflows.Workflow{
		ContainerID:  mod.ID,
		Name:         "ZzFlow",
		WorkflowName: "Zz Flow",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{
			Activities: []workflows.WorkflowActivity{
				&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
				userTask,
				mfTask,
				&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}},
			},
		},
	}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzFlow" {
			got = w
			break
		}
	}
	if got == nil {
		t.Fatalf("workflow ZzFlow not found after create")
	}
	if got.Parameter == nil || got.Parameter.EntityRef != "MyFirstModule.Ctx" {
		t.Errorf("parameter entity not round-tripped: %+v", got.Parameter)
	}
	if got.Flow == nil || len(got.Flow.Activities) != 4 {
		t.Fatalf("flow activities = %d, want 4", len(activitiesOf(got)))
	}
}

func activitiesOf(w *workflows.Workflow) []workflows.WorkflowActivity {
	if w.Flow == nil {
		return nil
	}
	return w.Flow.Activities
}

// TestWorkflowSimpleActivities_ReconstructedTyped guards Bug 11b: the modelsdk
// reader used to decode start/end, jump-to, and the wait activities as
// GenericWorkflowActivity (no genWf struct), so DESCRIBE rendered them as
// non-round-trippable "-- [Workflows$…]" comments. They must now round-trip as
// their typed structs with fields (jump target, timer delay) preserved.
func TestWorkflowSimpleActivities_ReconstructedTyped(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	const delay = "addHours([%CurrentDateTime%], 1)"
	wf := &workflows.Workflow{
		ContainerID: mod.ID,
		Name:        "ZzSimpleFlow",
		Parameter:   &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
			&workflows.WaitForTimerActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Wait"}, DelayExpression: delay},
			&workflows.WaitForNotificationActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "WaitNotif"}},
			&workflows.JumpToActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Jump", Caption: "Jump"}, TargetActivity: "Start"},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}},
		}},
	}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzSimpleFlow" {
			got = w
			break
		}
	}
	if got == nil || got.Flow == nil {
		t.Fatalf("workflow ZzSimpleFlow not found after create")
	}

	var jump *workflows.JumpToActivity
	var timer *workflows.WaitForTimerActivity
	var sawStart, sawEnd, sawNotif bool
	for _, a := range got.Flow.Activities {
		switch x := a.(type) {
		case *workflows.StartWorkflowActivity:
			sawStart = true
		case *workflows.EndWorkflowActivity:
			sawEnd = true
		case *workflows.WaitForNotificationActivity:
			sawNotif = true
		case *workflows.JumpToActivity:
			jump = x
		case *workflows.WaitForTimerActivity:
			timer = x
		case *workflows.GenericWorkflowActivity:
			t.Errorf("activity decoded as GenericWorkflowActivity (%s) — reader gap (Bug 11b)", x.TypeString)
		}
	}
	if !sawStart || !sawEnd || !sawNotif {
		t.Errorf("start/end/wait-notification not all reconstructed typed (start=%v end=%v notif=%v)", sawStart, sawEnd, sawNotif)
	}
	if jump == nil {
		t.Fatal("JumpToActivity decoded as a non-typed activity")
	}
	if jump.TargetActivity != "Start" {
		t.Errorf("jump target = %q, want %q", jump.TargetActivity, "Start")
	}
	if timer == nil {
		t.Fatal("WaitForTimerActivity decoded as a non-typed activity")
	}
	if timer.DelayExpression != delay {
		t.Errorf("timer delay = %q, want %q", timer.DelayExpression, delay)
	}
}
