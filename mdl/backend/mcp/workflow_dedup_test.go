// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #945. A CALL MICROFLOW activity is named after its target microflow, so
// inserting a call to a microflow the workflow already calls produces a
// colliding name before anything looks at it. Measured against live Studio Pro
// (11.x, PED 1.0.0): ped_update_document accepts the duplicate with SUCCESS and
// does NOT auto-rename, and ped_check_errors then reports
// "Duplicate name 'X'. (at locations: /flow/activities/1, /flow/activities/2)".
// A `set` on the activity's /name is refused ("Element type does not support
// renaming"), so the name has to be right at add time — there is no repair.
func TestWFInsertAfterActivity_DeduplicatesName(t *testing.T) {
	f, m := wfMutatorFake(t)
	dup := &workflows.CallMicroflowTask{Microflow: "M.ACT_ReviewOrder"}
	dup.Name = "ReviewOrder" // the fake flow already has a "ReviewOrder" user task
	dup.Caption = "Review the order"

	if err := m.InsertAfterActivity("ReviewOrder", 0, []workflows.WorkflowActivity{dup}); err != nil {
		t.Fatalf("InsertAfterActivity: %v", err)
	}

	ops := wfUpdateOps(t, f)
	if !strings.Contains(ops, `"name":"ReviewOrder_2"`) {
		t.Errorf("inserted activity kept the colliding name (CE0495 in Studio Pro): %s", ops)
	}
}

// Control: a name that collides with nothing must be sent through untouched.
func TestWFInsertAfterActivity_LeavesFreeNameAlone(t *testing.T) {
	f, m := wfMutatorFake(t)
	fresh := &workflows.CallMicroflowTask{Microflow: "M.ACT_Ship"}
	fresh.Name = "ShipOrder"

	if err := m.InsertAfterActivity("ReviewOrder", 0, []workflows.WorkflowActivity{fresh}); err != nil {
		t.Fatalf("InsertAfterActivity: %v", err)
	}

	ops := wfUpdateOps(t, f)
	if !strings.Contains(ops, `"name":"ShipOrder"`) || strings.Contains(ops, `"ShipOrder_2"`) {
		t.Errorf("a free name must not be renamed: %s", ops)
	}
}

// Nested names count too: uniqueness is workflow-wide, and the fake flow's
// "Decide" lives at the top level while the new activity goes into an outcome
// sub-flow.
func TestWFInsertOutcome_DeduplicatesAgainstWholeWorkflow(t *testing.T) {
	f, m := wfMutatorFake(t)
	nested := &workflows.CallMicroflowTask{Microflow: "M.ACT_Decide"}
	nested.Name = "Decide"

	if err := m.InsertOutcome("ReviewOrder", 0, "Escalate", []workflows.WorkflowActivity{nested}); err != nil {
		t.Fatalf("InsertOutcome: %v", err)
	}

	ops := wfUpdateOps(t, f)
	if !strings.Contains(ops, `"name":"Decide_2"`) {
		t.Errorf("sub-flow activity kept a name taken at the top level: %s", ops)
	}
}

// Two calls to the same microflow inserted in one statement collide with each
// other, not just with what is stored.
func TestWFInsertAfterActivity_DeduplicatesWithinTheBatch(t *testing.T) {
	f, m := wfMutatorFake(t)
	a := &workflows.CallMicroflowTask{Microflow: "M.ACT_Notify"}
	a.Name = "Notify"
	b := &workflows.CallMicroflowTask{Microflow: "M.ACT_Notify"}
	b.Name = "Notify"

	if err := m.InsertAfterActivity("ReviewOrder", 0, []workflows.WorkflowActivity{a, b}); err != nil {
		t.Fatalf("InsertAfterActivity: %v", err)
	}

	ops := wfUpdateOps(t, f)
	if !strings.Contains(ops, `"name":"Notify"`) || !strings.Contains(ops, `"name":"Notify_2"`) {
		t.Errorf("batch inserts collided with each other: %s", ops)
	}
}

// ped_update_document reports only op-level failures — a duplicate name comes
// back as SUCCESS. ped_check_errors is the only thing that sees it, so an ALTER
// that touched nothing but activities must still reach it.
func TestWFSave_ValidatesAfterActivityOnlyAlter(t *testing.T) {
	f, m := wfMutatorFake(t)
	act := &workflows.CallMicroflowTask{Microflow: "M.ACT_Ship"}
	act.Name = "ShipOrder"
	if err := m.InsertAfterActivity("ReviewOrder", 0, []workflows.WorkflowActivity{act}); err != nil {
		t.Fatalf("InsertAfterActivity: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.callByName("ped_check_errors"); !ok {
		t.Error("an activity-only ALTER never validated: ped_check_errors was not called")
	}
}

// Control: a mutator that did nothing must not call PED at all.
func TestWFSave_NoOpDoesNotValidate(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.callByName("ped_check_errors"); ok {
		t.Error("Save with no ops must not call ped_check_errors")
	}
}

// A DROP-only ALTER is a write too, so it validates as well.
func TestWFSave_ValidatesAfterDropOnlyAlter(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.DropActivity("Decide", 0); err != nil {
		t.Fatalf("DropActivity: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := f.callByName("ped_check_errors"); !ok {
		t.Error("a drop-only ALTER never validated: ped_check_errors was not called")
	}
}

// Issue #944. A REPLACE whose replacement reuses the original's name is an
// in-place edit, not a collision — the original leaves in the same PED call.
// PR #204 propagated the file backends' rename into this backend as deliberate
// "parity"; it was neither deliberate nor correct.
func TestWFReplaceActivity_SameNameKeepsName(t *testing.T) {
	f, m := wfMutatorFake(t)
	rep := &workflows.CallMicroflowTask{Microflow: "M.ACT_Review"}
	rep.Name = "ReviewOrder" // the same activity, edited in place
	rep.Caption = "Review the order (v2)"

	if err := m.ReplaceActivity("ReviewOrder", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	ops := wfUpdateOps(t, f)
	if !strings.Contains(ops, `"name":"ReviewOrder"`) || strings.Contains(ops, `"ReviewOrder_2"`) {
		t.Errorf("same-name replace renamed the activity: %s", ops)
	}
}

// Control: only the outgoing name is freed. Colliding with a surviving activity
// still dedupes.
func TestWFReplaceActivity_StillDedupesAgainstSurvivors(t *testing.T) {
	f, m := wfMutatorFake(t)
	rep := &workflows.CallMicroflowTask{Microflow: "M.ACT_Decide"}
	rep.Name = "Decide" // "Decide" is a different activity that survives

	if err := m.ReplaceActivity("ReviewOrder", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	if ops := wfUpdateOps(t, f); !strings.Contains(ops, `"name":"Decide_2"`) {
		t.Errorf("replacement colliding with a survivor must dedupe: %s", ops)
	}
}

// The reference may be the caption, so the freed name has to come from the
// resolved activity. The fake's "ReviewOrder" task has caption "Review the order".
func TestWFReplaceActivity_ResolvedByCaption(t *testing.T) {
	f, m := wfMutatorFake(t)
	rep := &workflows.CallMicroflowTask{Microflow: "M.ACT_Review"}
	rep.Name = "ReviewOrder"

	if err := m.ReplaceActivity("Review the order", 0, []workflows.WorkflowActivity{rep}); err != nil {
		t.Fatalf("ReplaceActivity: %v", err)
	}
	if ops := wfUpdateOps(t, f); !strings.Contains(ops, `"name":"ReviewOrder"`) || strings.Contains(ops, `"ReviewOrder_2"`) {
		t.Errorf("resolved-by-caption replace renamed the activity: %s", ops)
	}
}
