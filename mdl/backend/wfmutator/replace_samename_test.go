// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #944. REPLACE ACTIVITY edits one activity in place, so the replacement
// reusing the original's name is the normal case, not a collision — the original
// is on its way out. Deduplicating against a name pool that still contains it
// renamed every same-name replace to Name_2.
func TestWorkflowMutator_ReplaceActivity_SameNameKeepsName(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Original Caption A", "TaskA")
	other := makeWfActivity("Workflows$UserTask", "Original Caption B", "TaskB")
	m := newMutator(makeWorkflowDoc(act, other))

	rep := makeTestWorkflowActivity("TaskA", "Changed Caption A")
	if err := m.ReplaceActivity("TaskA", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}

	acts := getActivities(m.rawData)
	if got := bsonnav.DGetString(acts[0], "Name"); got != "TaskA" {
		t.Errorf("replaced activity renamed to %q; a same-name replace must keep the name", got)
	}
	if got := bsonnav.DGetString(acts[1], "Name"); got != "TaskB" {
		t.Errorf("untouched sibling = %q, want TaskB", got)
	}
}

// The rename compounded: every re-run of the same script suffixed again
// (TaskA -> TaskA_2 -> TaskA_2_2 -> ...), so an ALTER that changed nothing still
// mutated the model on every run — against the idempotence ADR-0008 promises.
func TestWorkflowMutator_ReplaceActivity_SameNameIsIdempotent(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Caption", "TaskA")
	m := newMutator(makeWorkflowDoc(act))

	for i := range 4 {
		rep := makeTestWorkflowActivity("TaskA", "Caption")
		if err := m.ReplaceActivity("TaskA", 0, []workflows.WorkflowActivity{rep}); err != nil {
			t.Fatalf("ReplaceActivity run %d: %v", i+1, err)
		}
		got := bsonnav.DGetString(getActivities(m.rawData)[0], "Name")
		if got != "TaskA" {
			t.Fatalf("after %d run(s) the name drifted to %q; the suffix accretes", i+1, got)
		}
	}
}

// Control: the exclusion must free ONLY the outgoing activity's name. A
// replacement colliding with a different, surviving activity still dedupes.
func TestWorkflowMutator_ReplaceActivity_StillDedupesAgainstOthers(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Caption A", "TaskA")
	other := makeWfActivity("Workflows$UserTask", "Caption B", "TaskB")
	m := newMutator(makeWorkflowDoc(act, other))

	// Replacing TaskA with something named TaskB collides with a survivor.
	rep := makeTestWorkflowActivity("TaskB", "Caption B")
	if err := m.ReplaceActivity("TaskA", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	acts := getActivities(m.rawData)
	if got := bsonnav.DGetString(acts[0], "Name"); got != "TaskB_2" {
		t.Errorf("replacement = %q, want TaskB_2 (TaskB survives, so it is still taken)", got)
	}
}

// The reference may be a caption, so the name to free has to come from the
// resolved activity rather than from the reference string.
func TestWorkflowMutator_ReplaceActivity_ResolvedByCaption(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review the order", "ReviewOrder")
	m := newMutator(makeWorkflowDoc(act))

	rep := makeTestWorkflowActivity("ReviewOrder", "Review the order (v2)")
	if err := m.ReplaceActivity("Review the order", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	if got := bsonnav.DGetString(getActivities(m.rawData)[0], "Name"); got != "ReviewOrder" {
		t.Errorf("resolved-by-caption replace renamed to %q, want ReviewOrder", got)
	}
}

// Multi-activity replace: the outgoing name is freed once, so the first
// replacement may take it and the rest dedupe among themselves.
func TestWorkflowMutator_ReplaceActivity_MultipleReplacements(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Caption", "TaskA")
	m := newMutator(makeWorkflowDoc(act))

	a := makeTestWorkflowActivity("TaskA", "A")
	b := makeTestWorkflowActivity("TaskA", "B")
	if err := m.ReplaceActivity("TaskA", 0, []workflows.WorkflowActivity{a, b}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	acts := getActivities(m.rawData)
	names := []string{bsonnav.DGetString(acts[0], "Name"), bsonnav.DGetString(acts[1], "Name")}
	if names[0] != "TaskA" || names[1] != "TaskA_2" {
		t.Errorf("names = %v, want [TaskA TaskA_2]", names)
	}
	if names[0] == names[1] {
		t.Errorf("both replacements got %q — a duplicate name is CE0495", fmt.Sprint(names[0]))
	}
}
