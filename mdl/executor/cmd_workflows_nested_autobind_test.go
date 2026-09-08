// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// newNestedAutoBindCtx builds an ExecContext whose backend knows one microflow,
// WF.ACT_Noop($Ctx), so autoBindCallMicroflow can resolve its parameters.
func newNestedAutoBindCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("WF")
	mf := mkMicroflow(mod.ID, "ACT_Noop")
	mf.Parameters = []*microflows.MicroflowParameter{{
		BaseElement: model.BaseElement{ID: nextID("param")},
		Name:        "Ctx",
	}}

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func mkCallMicroflowTask(name string) *workflows.CallMicroflowTask {
	task := &workflows.CallMicroflowTask{Microflow: "WF.ACT_Noop"}
	task.ID = model.ID(nextID("act"))
	task.Name = name
	task.Caption = name
	return task
}

func mkWfFlow(acts ...workflows.WorkflowActivity) *workflows.Flow {
	f := &workflows.Flow{Activities: acts}
	f.ID = model.ID(nextID("flow"))
	return f
}

// assertWired is the CE6685/CE6686 assertion: a call-microflow activity Mendix
// accepts carries one parameter mapping per target parameter and at least one
// outcome. Both are auto-generated; neither is written by the MDL statement.
func assertWired(t *testing.T, where string, task *workflows.CallMicroflowTask) {
	t.Helper()
	if len(task.ParameterMappings) != 1 {
		t.Errorf("%s: got %d parameter mappings, want 1 (CE6685)", where, len(task.ParameterMappings))
	} else if got := task.ParameterMappings[0].Expression; got != "$WorkflowContext" {
		t.Errorf("%s: parameter mapping expression = %q, want %q", where, got, "$WorkflowContext")
	}
	if len(task.Outcomes) == 0 {
		t.Errorf("%s: got 0 outcomes, want >= 1 (CE6686)", where)
	}
}

// TestAutoBindReachesNestedCallMicroflow guards ako/mxcli#417: a call-microflow
// activity nested inside a decision's ENUMERATION outcome (or a boundary event
// body) was never visited by autoBindActivitiesInFlow, so it reached Mendix with
// no parameter mappings and no outcomes — CE6685 + CE6686. The MAIN-flow control
// in the same table was always wired; that is what made the gap invisible.
func TestAutoBindReachesNestedCallMicroflow(t *testing.T) {
	cases := []struct {
		name  string
		build func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity
	}{
		{"decision/enum outcome", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			out := &workflows.EnumerationValueConditionOutcome{Value: "WF.Status.OutcomeA", Flow: mkWfFlow(task)}
			out.ID = model.ID(nextID("out"))
			split := &workflows.ExclusiveSplitActivity{
				Expression: "$WorkflowContext/Status",
				Outcomes:   []workflows.ConditionOutcome{out},
			}
			split.ID = model.ID(nextID("act"))
			split.Name = "decision1"
			return split
		}},
		{"decision/boolean outcome", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			out := &workflows.BooleanConditionOutcome{Value: true, Flow: mkWfFlow(task)}
			out.ID = model.ID(nextID("out"))
			split := &workflows.ExclusiveSplitActivity{Outcomes: []workflows.ConditionOutcome{out}}
			split.ID = model.ID(nextID("act"))
			split.Name = "decision1"
			return split
		}},
		{"call microflow/enum outcome", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			out := &workflows.EnumerationValueConditionOutcome{Value: "WF.Status.OutcomeA", Flow: mkWfFlow(task)}
			out.ID = model.ID(nextID("out"))
			outer := mkCallMicroflowTask("outerCall")
			outer.Outcomes = []workflows.ConditionOutcome{out}
			return outer
		}},
		{"user task outcome", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			out := &workflows.UserTaskOutcome{Value: "Approve", Flow: mkWfFlow(task)}
			out.ID = model.ID(nextID("out"))
			ut := &workflows.UserTask{Outcomes: []*workflows.UserTaskOutcome{out}}
			ut.ID = model.ID(nextID("act"))
			ut.Name = "userTask1"
			return ut
		}},
		{"parallel split path", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			out := &workflows.ParallelSplitOutcome{Flow: mkWfFlow(task)}
			out.ID = model.ID(nextID("out"))
			ps := &workflows.ParallelSplitActivity{Outcomes: []*workflows.ParallelSplitOutcome{out}}
			ps.ID = model.ID(nextID("act"))
			ps.Name = "split1"
			return ps
		}},
		{"user task boundary event", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			be := &workflows.BoundaryEvent{EventType: "InterruptingTimer", Flow: mkWfFlow(task)}
			be.ID = model.ID(nextID("be"))
			ut := &workflows.UserTask{BoundaryEvents: []*workflows.BoundaryEvent{be}}
			ut.ID = model.ID(nextID("act"))
			ut.Name = "userTask1"
			return ut
		}},
		{"call microflow boundary event", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			be := &workflows.BoundaryEvent{EventType: "InterruptingTimer", Flow: mkWfFlow(task)}
			be.ID = model.ID(nextID("be"))
			outer := mkCallMicroflowTask("outerCall")
			outer.BoundaryEvents = []*workflows.BoundaryEvent{be}
			return outer
		}},
		{"wait for notification boundary event", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			be := &workflows.BoundaryEvent{EventType: "InterruptingTimer", Flow: mkWfFlow(task)}
			be.ID = model.ID(nextID("be"))
			w := &workflows.WaitForNotificationActivity{BoundaryEvents: []*workflows.BoundaryEvent{be}}
			w.ID = model.ID(nextID("act"))
			w.Name = "wait1"
			return w
		}},
		{"call workflow boundary event", func(task *workflows.CallMicroflowTask) workflows.WorkflowActivity {
			be := &workflows.BoundaryEvent{EventType: "InterruptingTimer", Flow: mkWfFlow(task)}
			be.ID = model.ID(nextID("be"))
			cw := &workflows.CallWorkflowActivity{Workflow: "WF.Other"}
			cw.ID = model.ID(nextID("act"))
			cw.Name = "callWorkflow1"
			cw.BoundaryEvents = []*workflows.BoundaryEvent{be}
			return cw
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newNestedAutoBindCtx(t)

			// The MAIN-flow control: the identical activity, unnested. It was
			// always wired, so a failure here means the test setup is wrong,
			// not that the nesting gap is real.
			control := mkCallMicroflowTask("controlCall")
			nested := mkCallMicroflowTask("nestedCall")

			autoBindWorkflowParameters(ctx, []workflows.WorkflowActivity{control, tc.build(nested)}, "Ctx")

			assertWired(t, "MAIN flow control", control)
			assertWired(t, "nested in "+tc.name, nested)
		})
	}
}
