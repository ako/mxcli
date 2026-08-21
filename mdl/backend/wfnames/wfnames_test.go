// SPDX-License-Identifier: Apache-2.0

package wfnames

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

func TestUnique(t *testing.T) {
	taken := map[string]bool{}
	cases := []struct{ in, want string }{
		{"Approve", "Approve"},
		{"Approve", "Approve_2"},
		{"Approve", "Approve_3"},
		{"Reject", "Reject"},
		{"", ""}, // an empty name is left alone and never recorded
		{"", ""},
	}
	for _, c := range cases {
		if got := Unique(c.in, taken); got != c.want {
			t.Errorf("Unique(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if taken[""] {
		t.Error("the empty name must not be recorded as taken")
	}
}

// A seen-count would hand out a name the workflow already contains: A, A, A_2
// counts its way to A, A_2, A_2. The taken-set must skip to A_3.
func TestUnique_SkipsNameAlreadyPresent(t *testing.T) {
	taken := map[string]bool{"Approve_2": true}
	if got := Unique("Approve", taken); got != "Approve" {
		t.Fatalf("first Approve = %q", got)
	}
	if got := Unique("Approve", taken); got != "Approve_3" {
		t.Errorf("second Approve = %q, want Approve_3 (Approve_2 was already taken)", got)
	}
}

func TestDedup_SeededWithExistingNames(t *testing.T) {
	a := &workflows.CallMicroflowTask{Microflow: "M.ACT_Do"}
	a.Name = "ACT_Do"
	b := &workflows.CallMicroflowTask{Microflow: "M.ACT_Do"}
	b.Name = "ACT_Do"

	Dedup([]workflows.WorkflowActivity{a, b}, map[string]bool{"ACT_Do": true})

	if a.Name != "ACT_Do_2" || b.Name != "ACT_Do_3" {
		t.Errorf("names = %q, %q; want ACT_Do_2, ACT_Do_3", a.Name, b.Name)
	}
}

// Uniqueness is workflow-wide, so an inserted activity's own sub-flows have to
// be walked too.
func TestDedup_RecursesIntoSubFlows(t *testing.T) {
	nested := &workflows.CallMicroflowTask{Microflow: "M.ACT_Do"}
	nested.Name = "ACT_Do"
	deeper := &workflows.WaitForTimerActivity{}
	deeper.Name = "ACT_Do"
	nestedBoundary := &workflows.BoundaryEvent{
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{deeper}},
	}
	task := &workflows.UserTask{
		Outcomes: []*workflows.UserTaskOutcome{
			{Value: "Approve", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{nested}}},
		},
		BoundaryEvents: []*workflows.BoundaryEvent{nestedBoundary},
	}
	task.Name = "ACT_Do"

	Dedup([]workflows.WorkflowActivity{task}, map[string]bool{})

	if task.Name != "ACT_Do" || nested.Name != "ACT_Do_2" || deeper.Name != "ACT_Do_3" {
		t.Errorf("names = %q, %q, %q; want ACT_Do, ACT_Do_2, ACT_Do_3", task.Name, nested.Name, deeper.Name)
	}
}

func TestSubFlows_OutcomesBeforeBoundaryEvents(t *testing.T) {
	outFlow := &workflows.Flow{}
	beFlow := &workflows.Flow{}
	call := &workflows.CallMicroflowTask{
		Outcomes:       []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{Flow: outFlow}},
		BoundaryEvents: []*workflows.BoundaryEvent{{Flow: beFlow}},
	}
	got := SubFlows(call)
	if len(got) != 2 || got[0] != outFlow || got[1] != beFlow {
		t.Errorf("SubFlows order = %v; want [outcome, boundaryEvent]", got)
	}
}

// A nil flow must not become a nil entry the walker then dereferences.
func TestSubFlows_SkipsAbsentFlows(t *testing.T) {
	call := &workflows.CallMicroflowTask{
		Outcomes:       []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{}},
		BoundaryEvents: []*workflows.BoundaryEvent{{}},
	}
	if got := SubFlows(call); len(got) != 0 {
		t.Errorf("SubFlows = %v, want none", got)
	}
}
