// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1024 — `jump to A;` inside a boundary-event body wrote a Jump
// NAMED A instead of one targeting A:
//
//	[error] [CE0495] "Duplicate name 'A'." at User task 'A', Jump 'A'
//	[error] [CE6680] "The 'Target' property is required." at Jump 'A'
//
// Same root cause as #1005 (buildJumpTo named the jump after its target, so the
// target resolved to the jump itself); reported against v0.20.0, which predates
// that fix. The #1005 tests only put jumps in outcome flows — this pins the
// boundary-event body, which is the only way to end an interrupting boundary
// event's path besides `end workflow`.

// allActivities flattens the tree, boundary-event bodies included.
func allActivities(acts []workflows.WorkflowActivity) []workflows.WorkflowActivity {
	var out []workflows.WorkflowActivity
	for _, a := range acts {
		out = append(out, a)
		for _, f := range nestedFlows(a) {
			out = append(out, allActivities(f.Activities)...)
		}
	}
	return out
}

func TestJumpTo_InBoundaryEventBody(t *testing.T) {
	for name, src := range map[string]string{
		"user task": `create workflow Probe.WF
  parameter $WorkflowContext: Probe.Ctx
begin
  user task A 'A' page Probe.WF_TaskPage outcomes 'Done' { }
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' { jump to A; };
end workflow;`,
		"wait for notification": `create workflow Probe.WF
  parameter $WorkflowContext: Probe.Ctx
begin
  wait for notification A
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' { jump to A; };
end workflow;`,
	} {
		t.Run(name, func(t *testing.T) {
			var jump *workflows.JumpToActivity
			seen := map[string]int{}
			for _, a := range allActivities(buildWorkflowFrom(t, src)) {
				if j, ok := a.(*workflows.JumpToActivity); ok {
					jump = j
				}
				if n := a.GetName(); n != "" {
					seen[n]++
				}
			}
			if jump == nil {
				t.Fatal("no jump activity was built inside the boundary-event body")
			}
			if jump.TargetActivity != "A" {
				t.Errorf("TargetActivity = %q, want A (CE6680)", jump.TargetActivity)
			}
			if jump.Name == jump.TargetActivity {
				t.Errorf("jump is named after its target (%q) — it targets itself", jump.Name)
			}
			for n, c := range seen {
				if c > 1 {
					t.Errorf("Duplicate name %q (%d activities) — CE0495", n, c)
				}
			}
		})
	}
}
