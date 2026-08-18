// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FINDINGS #24: Mendix can run a Call microflow and a Call Java action activity
// on a task queue, and MDL could express it on neither — the gen setters existed
// and nothing called them. These pin the authoring path end to end within the
// executor: grammar → AST → builder → semantic model.
//
// The finding proposed writing the call's `Queue` string. That property is
// inert (see queueSettingsToGen); the binding mxbuild reads is the
// Queues$QueueSettings child, which is what these assert.

func TestCallMicroflow_InQueue_BuildsQueueSettings(t *testing.T) {
	fb := &flowBuilder{}
	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "Q", Name: "ACT_Work"},
		Queue:         &ast.QualifiedName{Module: "Q", Name: "RefreshQueue"},
	})

	act := lastCallAction[*microflows.MicroflowCallAction](t, fb)
	if act.MicroflowCall == nil {
		t.Fatal("no MicroflowCall built")
	}
	qs := act.MicroflowCall.QueueSettings
	if qs == nil {
		t.Fatal("IN QUEUE produced no QueueSettings — the binding is dropped")
	}
	if qs.Queue != "Q.RefreshQueue" {
		t.Errorf("QueueSettings.Queue = %q, want %q", qs.Queue, "Q.RefreshQueue")
	}
	if qs.ID == "" {
		t.Error("QueueSettings has no $ID")
	}
}

func TestCallJavaAction_InQueue_BuildsQueueSettings(t *testing.T) {
	fb := &flowBuilder{}
	fb.addCallJavaActionAction(&ast.CallJavaActionStmt{
		ActionName: ast.QualifiedName{Module: "Q", Name: "RefreshData"},
		Queue:      &ast.QualifiedName{Module: "Q", Name: "RefreshQueue"},
	})

	act := lastCallAction[*microflows.JavaActionCallAction](t, fb)
	if act.QueueSettings == nil {
		t.Fatal("IN QUEUE produced no QueueSettings — the binding is dropped")
	}
	if act.QueueSettings.Queue != "Q.RefreshQueue" {
		t.Errorf("QueueSettings.Queue = %q, want %q", act.QueueSettings.Queue, "Q.RefreshQueue")
	}
}

// An unqueued call must keep writing no QueueSettings at all: the call type's
// registered NullFields default serializes it as null, which is what Studio Pro
// stores and what every existing microflow round-trips as.
func TestCallMicroflow_WithoutQueue_HasNoQueueSettings(t *testing.T) {
	fb := &flowBuilder{}
	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "Q", Name: "ACT_Work"},
	})
	act := lastCallAction[*microflows.MicroflowCallAction](t, fb)
	if act.MicroflowCall.QueueSettings != nil {
		t.Fatalf("unqueued call gained a QueueSettings: %+v", act.MicroflowCall.QueueSettings)
	}
}

// lastCallAction returns the Action of the last activity the builder appended.
func lastCallAction[T microflows.MicroflowAction](t *testing.T, fb *flowBuilder) T {
	t.Helper()
	var zero T
	if len(fb.objects) == 0 {
		t.Fatal("builder appended no objects")
		return zero
	}
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("last object is %T, want *microflows.ActionActivity", fb.objects[len(fb.objects)-1])
		return zero
	}
	act, ok := activity.Action.(T)
	if !ok {
		t.Fatalf("action is %T, want %T", activity.Action, zero)
		return zero
	}
	return act
}

// TestFormatAction_RendersInQueue covers the DESCRIBE half. Without it a
// describe of a queued microflow emits a script whose re-execution silently
// unqueues the call — and, because the rewrite guard now allows a script that
// restates every stored queue, that script would be accepted.
func TestFormatAction_RendersInQueue(t *testing.T) {
	mfAct := &microflows.MicroflowCallAction{
		MicroflowCall: &microflows.MicroflowCall{
			Microflow:     "Q.ACT_Work",
			QueueSettings: &microflows.QueueSettings{Queue: "Q.RefreshQueue"},
		},
	}
	jaAct := &microflows.JavaActionCallAction{
		JavaAction:    "Q.RefreshData",
		QueueSettings: &microflows.QueueSettings{Queue: "Q.RefreshQueue"},
	}

	for name, act := range map[string]microflows.MicroflowAction{"microflow": mfAct, "javaaction": jaAct} {
		t.Run(name, func(t *testing.T) {
			got := formatAction(nil, act, nil, nil)
			if !strings.Contains(got, "in queue Q.RefreshQueue") {
				t.Fatalf("formatted as %q, want it to carry `in queue Q.RefreshQueue`", got)
			}
		})
	}

	// An unqueued call must not grow the clause.
	plain := &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{Microflow: "Q.ACT_Work"}}
	if got := formatAction(nil, plain, nil, nil); strings.Contains(got, "in queue") {
		t.Fatalf("unqueued call formatted as %q", got)
	}
}
