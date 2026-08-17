// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// storedCall builds the shape a stored MicroflowCall has once a task queue is
// bound to it in Studio Pro. The binding lives two levels down (Queues$QueueSettings
// inside the call's QueueSettings property), so the walk has to be recursive.
func storedCall(queueSettings map[string]any, topLevelQueue any) map[string]any {
	return map[string]any{
		"$Type": "Microflows$Microflow",
		"ObjectCollection": map[string]any{
			"Objects": []any{
				map[string]any{
					"$Type": "Microflows$ActionActivity",
					"Action": map[string]any{
						"$Type":         "Microflows$MicroflowCall",
						"Microflow":     "Q.Target",
						"Queue":         topLevelQueue,
						"QueueSettings": queueSettings,
					},
				},
			},
		},
	}
}

func TestQueuedCallTargets_FindsQueueSettings(t *testing.T) {
	got := queuedCallTargets(storedCall(map[string]any{
		"$Type": "Queues$QueueSettings",
		"Queue": "Q.MyQueue",
	}, nil))
	if len(got) != 1 || got[0] != "Q.MyQueue" {
		t.Fatalf("queuedCallTargets = %v, want [Q.MyQueue]", got)
	}
}

func TestQueuedCallTargets_NoBinding(t *testing.T) {
	if got := queuedCallTargets(storedCall(nil, nil)); len(got) != 0 {
		t.Fatalf("queuedCallTargets = %v, want none for an unqueued call", got)
	}
}

// A call can carry a bare Queue with QueueSettings null. Measured on 11.13 that
// is inert (mx check does not complain), but it is still authored state, and a
// rewrite would drop it.
func TestQueuedCallTargets_BareQueue(t *testing.T) {
	got := queuedCallTargets(storedCall(nil, "Q.MyQueue"))
	if len(got) != 1 || got[0] != "Q.MyQueue" {
		t.Fatalf("queuedCallTargets = %v, want [Q.MyQueue]", got)
	}
}

func TestQueuedCallTargets_Dedupes(t *testing.T) {
	doc := map[string]any{"Objects": []any{
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.A"}},
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.A"}},
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.B"}},
	}}
	got := queuedCallTargets(doc)
	if len(got) != 2 {
		t.Fatalf("queuedCallTargets = %v, want 2 distinct queues", got)
	}
}

// TestCheckNoQueuedCalls_Refuses is the guard against the data loss itself: a
// CREATE OR REPLACE of a microflow with a queued call used to succeed and write
// QueueSettings back as null, so mx check went from CE1613 to 0 errors by
// deleting the user's configuration.
func TestCheckNoQueuedCalls_Refuses(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil)
	if err == nil {
		t.Fatal("expected a refusal for a microflow with a queued call")
	}
	msg := err.Error()
	for _, want := range []string{"Q.ACT_Caller", "Q.MyQueue", "task queue"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckNoQueuedCalls_AllowsUnqueued(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(nil, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil); err != nil {
		t.Fatalf("unqueued microflow must still be rewritable: %v", err)
	}
}

// An unreadable unit is not this guard's business — the rewrite path reports its
// own errors, and failing here would block writes for an unrelated reason.
func TestCheckNoQueuedCalls_UnreadableUnitDoesNotBlock(t *testing.T) {
	ctx, _ := newMockCtx(t) // default mock: GetRawUnit is not configured, so it errors
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil); err != nil {
		t.Fatalf("unreadable unit must not block the write: %v", err)
	}
}

// TestCreateOrModifyMicroflow_RefusesQueuedCall drives the guard through the
// real CREATE OR MODIFY path. This is the test that fails if the call site in
// execCreateMicroflow is removed — checkNoQueuedCalls passing on its own proves
// nothing about whether anything calls it.
func TestCreateOrModifyMicroflow_RefusesQueuedCall(t *testing.T) {
	mod := mkModule("Q")
	h := mkHierarchy(mod)

	existing := &microflows.Microflow{Name: "ACT_Caller"}
	existing.ID = "mf-existing"
	existing.ContainerID = mod.ID

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{existing}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
		UpdateMicroflowFunc: func(mf *microflows.Microflow) error {
			t.Error("UpdateMicroflow must not run: the rewrite would drop the queue binding")
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateMicroflow(ctx, &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		CreateOrModify: true,
	})
	if err == nil {
		t.Fatal("expected a refusal, got a successful rewrite")
	}
	if !strings.Contains(err.Error(), "Q.MyQueue") {
		t.Errorf("error should name the queue that would be lost:\n%s", err)
	}
}

// `IN QUEUE` makes the binding authorable, so the guard changes shape: a script
// that restates every stored queue is the normal way to edit a microflow with a
// queued call and must go through. Before the clause existed, every such rewrite
// was refused because there was nothing to restate it with.
func TestCheckNoQueuedCalls_AllowsWhenScriptRestatesQueue(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	restating := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
				Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
			},
		},
	}
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", restating); err != nil {
		t.Fatalf("a script that restates the queue must be allowed: %v", err)
	}

	// Dropping it is still refused, and the message must name the clause that
	// fixes it — the whole point of the guard is that the loss is invisible.
	dropping := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"}},
		},
	}
	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", dropping)
	if err == nil {
		t.Fatal("a rewrite that drops the binding must still be refused")
	}
	for _, want := range []string{"Q.MyQueue", "IN QUEUE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q:\n%s", want, err.Error())
		}
	}
}

// A call nested inside IF/LOOP/error-handler bodies still counts as restating.
// The walk is reflective precisely so a newly added nesting cannot silently stop
// being searched — a hand-written switch would.
func TestAuthoredQueueTargets_FindsNestedCalls(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Body: []ast.MicroflowStatement{
			&ast.IfStmt{
				ThenBody: []ast.MicroflowStatement{
					&ast.LoopStmt{
						Body: []ast.MicroflowStatement{
							&ast.CallJavaActionStmt{
								ActionName: ast.QualifiedName{Module: "Q", Name: "Work"},
								Queue:      &ast.QualifiedName{Module: "Q", Name: "Deep"},
							},
						},
					},
				},
			},
		},
	}
	got := authoredQueueTargets(stmt)
	if !got["q.deep"] {
		t.Fatalf("nested IN QUEUE not found: %v", got)
	}
}

// A stored retry policy has no MDL spelling, so restating the queue is not
// enough — the rewrite would reset the retry. That must still refuse.
func TestCheckNoQueuedCalls_RefusesStoredRetry(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{
				"$Type": "Queues$QueueSettings",
				"Queue": "Q.MyQueue",
				"Retry": map[string]any{"$Type": "Queues$QueueFixedRetry", "Retries": 3},
			}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	restating := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
				Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
			},
		},
	}
	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", restating)
	if err == nil {
		t.Fatal("a stored retry policy must refuse the rewrite even when the queue is restated")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error should name the retry:\n%s", err.Error())
	}
}
