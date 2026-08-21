// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// storedWorkflowCtx returns a context whose raw stored workflow unit carries the
// given nested $Type nodes, so the guard has something to find.
func storedWorkflowCtx(t *testing.T, nested ...string) *ExecContext {
	t.Helper()
	acts := make([]any, 0, len(nested))
	for _, ty := range nested {
		acts = append(acts, map[string]any{"$Type": ty})
	}
	raw := map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{
			"$Type": "Workflows$Flow",
			"Activities": []any{
				map[string]any{
					"$Type":          "Workflows$CallMicroflowTask",
					"BoundaryEvents": acts,
				},
			},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc:  func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func parseWorkflowStmt(t *testing.T, src string) *ast.CreateWorkflowStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("statement is %T", prog.Statements[0])
	}
	return s
}

const wfNoBoundary = `create or replace workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step';
end workflow;`

const wfWithBoundary = `create or replace workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    boundary event interrupting timer 'addHours([%CurrentDateTime%], 2)' {
      call microflow M.ACT_Escalate;
    };
end workflow;`

// Issue #948. A rewrite rebuilds the workflow from the statement, so a stored
// boundary event the script does not restate is deleted along with its handler
// flow — measured 1 -> 0 while exec reported success.
func TestWorkflowRewrite_RefusesDroppingBoundaryEvent(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingTimerBoundaryEvent")
	err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary))
	if err == nil {
		t.Fatal("a rewrite that drops a stored boundary event was allowed")
	}
	if !strings.Contains(err.Error(), "boundary event") {
		t.Errorf("error should name the construct: %v", err)
	}
}

// Control: boundary events ARE authorable, so restating them is the normal way
// to edit such a workflow and must pass.
func TestWorkflowRewrite_AllowsRestatedBoundaryEvent(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingTimerBoundaryEvent")
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfWithBoundary)); err != nil {
		t.Errorf("a rewrite that restates the boundary event must be allowed: %v", err)
	}
}

// Control: a workflow with nothing to lose is never blocked.
func TestWorkflowRewrite_AllowsWhenNothingStored(t *testing.T) {
	ctx := storedWorkflowCtx(t)
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err != nil {
		t.Errorf("rewrite of a workflow with no stored constructs must be allowed: %v", err)
	}
}

// An event sub-process cannot be expressed in MDL at all, so restating is not an
// option and any stored one refuses the rewrite outright.
func TestWorkflowRewrite_RefusesEventSubProcessUnconditionally(t *testing.T) {
	ctx := storedWorkflowCtx(t, "Workflows$InterruptingNotificationEventSubProcessStartActivity")
	for name, src := range map[string]string{"restated": wfWithBoundary, "not restated": wfNoBoundary} {
		err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, src))
		if err == nil {
			t.Errorf("%s: a stored event sub-process must refuse the rewrite", name)
		} else if !strings.Contains(err.Error(), "event sub-process") {
			t.Errorf("%s: error should name the construct: %v", name, err)
		}
	}
}

// The guard must not depend on the semantic reader: that reader is what was
// blind here, and one sharing its blind spot cannot see what it protects. All
// three timer variants are matched by substring so a later one is caught too.
func TestWorkflowRewrite_MatchesEveryTimerVariant(t *testing.T) {
	for _, ty := range []string{
		"Workflows$InterruptingTimerBoundaryEvent",
		"Workflows$NonInterruptingTimerBoundaryEvent",
		"Workflows$TimerBoundaryEvent",
		"Workflows$SomeFutureBoundaryEvent",
	} {
		ctx := storedWorkflowCtx(t, ty)
		if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err == nil {
			t.Errorf("%s was not detected", ty)
		}
	}
}

// An unreadable stored unit is not this guard's business — the rewrite path
// reports its own errors, and failing here would block edits on a bad read.
func TestWorkflowRewrite_UnreadableUnitDoesNotBlock(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := checkNoDroppedWorkflowConstructs(ctx, "wf1", "M.W", parseWorkflowStmt(t, wfNoBoundary)); err != nil {
		t.Errorf("an unreadable unit must not block the rewrite: %v", err)
	}
}

// DESCRIBE emits MDL that must re-parse. The call-microflow describer emitted
// boundary events BEFORE outcomes, which the grammar rejects
// (workflowCallMicroflowStmt: … OUTCOMES? BOUNDARY EVENT?) — "mismatched input
// 'outcomes' expecting ';'". It was invisible while the default engine could not
// read boundary events back at all.
func TestWorkflowDescribe_BoundaryEventAfterOutcomesReparses(t *testing.T) {
	src := `create workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    outcomes
      DEFAULT -> { }
    boundary event interrupting timer 'addHours([%CurrentDateTime%], 2)' {
      call microflow M.ACT_Escalate;
    };
end workflow;`
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Fatalf("outcomes-then-boundary-event must parse: %v", errs)
	}

	// The order DESCRIBE used to emit: the grammar rejects it, which is what
	// made the round trip fail.
	bad := `create workflow M.W parameter $C: M.Ctx
begin
  call microflow M.ACT_Step comment 'Step'
    boundary event interrupting timer 'x' { }
    outcomes
      DEFAULT -> { };
end workflow;`
	if _, errs := visitor.Build(bad); len(errs) == 0 {
		t.Error("boundary-event-then-outcomes should NOT parse; if the grammar now " +
			"accepts both orders this test's premise is stale, but the describer " +
			"should still emit the documented order")
	}
}
