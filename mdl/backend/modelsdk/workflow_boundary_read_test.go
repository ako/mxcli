// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #948. The default engine's workflow reader had no boundary-event support
// at all, while the legacy engine's parser has had it all along. Both engines
// WRITE them, so a boundary event mxcli itself had just written read back as
// absent: DESCRIBE rendered nothing, and a describe -> edit -> re-exec round trip
// silently dropped the timer, its handler flow and the jump inside it.
func TestWorkflowRead_BoundaryEventsRoundTrip(t *testing.T) {
	inner := &workflows.CallMicroflowTask{Microflow: "M.ACT_Escalate"}
	inner.Name = "ACT_Escalate"
	jump := &workflows.JumpToActivity{TargetActivity: "Review"}
	jump.Name = "back"

	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"
	call.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "InterruptingTimer",
		TimerDelay: "addHours([%CurrentDateTime%], 2)",
		Caption:    "escalate",
		Flow:       &workflows.Flow{Activities: []workflows.WorkflowActivity{inner, jump}},
	}}

	got := roundTripWorkflowActivity(t, call)

	rt, ok := got.(*workflows.CallMicroflowTask)
	if !ok {
		t.Fatalf("round-tripped to %T, want *workflows.CallMicroflowTask", got)
	}
	if len(rt.BoundaryEvents) != 1 {
		t.Fatalf("boundary events after round trip = %d, want 1 (the reader dropped it)", len(rt.BoundaryEvents))
	}
	be := rt.BoundaryEvents[0]
	if be.EventType != "InterruptingTimer" {
		t.Errorf("EventType = %q, want InterruptingTimer", be.EventType)
	}
	if be.TimerDelay != "addHours([%CurrentDateTime%], 2)" {
		t.Errorf("TimerDelay = %q, want the delay expression", be.TimerDelay)
	}
	if be.Caption != "escalate" {
		t.Errorf("Caption = %q, want escalate", be.Caption)
	}
	if be.Flow == nil || len(be.Flow.Activities) != 2 {
		t.Fatalf("handler flow = %v, want 2 activities", be.Flow)
	}
	if _, ok := be.Flow.Activities[1].(*workflows.JumpToActivity); !ok {
		t.Errorf("handler activity[1] = %T, want *workflows.JumpToActivity", be.Flow.Activities[1])
	}
}

// A non-interrupting timer is a different $Type and must not be flattened onto
// the interrupting one.
func TestWorkflowRead_NonInterruptingBoundaryEvent(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"
	call.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "NonInterruptingTimer",
		TimerDelay: "[%CurrentDateTime%]",
	}}

	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallMicroflowTask)
	if !ok || len(rt.BoundaryEvents) != 1 {
		t.Fatalf("boundary event lost")
	}
	if got := rt.BoundaryEvents[0].EventType; got != "NonInterruptingTimer" {
		t.Errorf("EventType = %q, want NonInterruptingTimer", got)
	}
}

// Control: an activity with no boundary events must not gain an empty one.
func TestWorkflowRead_NoBoundaryEventsStaysEmpty(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"

	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallMicroflowTask)
	if !ok {
		t.Fatal("round trip failed")
	}
	if len(rt.BoundaryEvents) != 0 {
		t.Errorf("boundary events = %d, want 0", len(rt.BoundaryEvents))
	}
}

// roundTripWorkflowActivity encodes a semantic activity to gen through a
// workflow, runs it through the codec, and reads it back — the exact write→read
// path DESCRIBE and any CREATE OR REPLACE takes. Mirrors roundTripMicroflow.
func roundTripWorkflowActivity(t *testing.T, act workflows.WorkflowActivity) workflows.WorkflowActivity {
	t.Helper()
	wf := &workflows.Workflow{
		Name:      "WF",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{act}},
	}
	raw, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	dec, ok := el.(*genWf.Workflow)
	if !ok {
		t.Fatalf("decoded %T, want *genWf.Workflow", el)
	}
	f, ok := dec.Flow().(*genWf.Flow)
	if !ok || f == nil {
		t.Fatal("decoded workflow has no flow")
	}
	back := workflowFlowFromGen(f)
	if back == nil || len(back.Activities) == 0 {
		t.Fatal("round trip produced no activities")
	}
	return back.Activities[0]
}

// A USER TASK carries boundary events too, and it is the shape the workflow
// roundtrip integration tests use. It is worth pinning separately because the
// wiring is per-gen-type: genWf.UserTask (the older type) has no
// BoundaryEventsItems accessor at all, so the reader can only reach these
// through SingleUserTaskActivity / MultiUserTaskActivity. This asserts the
// encoder puts a user task somewhere the reader can actually see it.
func TestWorkflowRead_UserTaskBoundaryEvents(t *testing.T) {
	ut := &workflows.UserTask{Page: "M.TaskPage"}
	ut.Name = "Review"
	ut.Caption = "Review"
	ut.Outcomes = []*workflows.UserTaskOutcome{{Value: "Approve"}}
	ut.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "InterruptingTimer",
		TimerDelay: "${PT1H}",
	}}

	rt, ok := roundTripWorkflowActivity(t, ut).(*workflows.UserTask)
	if !ok {
		t.Fatalf("round-tripped to a different type")
	}
	if len(rt.BoundaryEvents) != 1 {
		t.Fatalf("user task boundary events = %d, want 1", len(rt.BoundaryEvents))
	}
	if got := rt.BoundaryEvents[0].TimerDelay; got != "${PT1H}" {
		t.Errorf("TimerDelay = %q, want ${PT1H}", got)
	}
}
