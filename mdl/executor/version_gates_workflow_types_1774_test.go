// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1774: the workflow activity and event TYPES older Mendix does not
// have are refused by `check -p` and by exec's pre-flight with MDL-VERSION01,
// before anything is written. The write path refuses the types themselves
// (canon.UndeclaredTypeError); these name the statement. Each floor is the
// type's own introduction in mendixmodelsdk 4.117.0:
//   - Workflows$WaitForNotificationActivity 10.0.0;
//   - Workflows$WaitForTimerActivity 10.6.0;
//   - Workflows$TimerBoundaryEvent 10.14.0 (a bare `boundary event timer`);
//   - Workflows$(Non)InterruptingTimerBoundaryEvent 10.20.0;
//   - Workflows$WorkflowDefinition(Name|Object)Selection 10.0.0 (lock/unlock).
func TestCheckVersionGates_WorkflowTypes(t *testing.T) {
	onTask := func(eventType string) *ast.WorkflowUserTaskNode {
		return &ast.WorkflowUserTaskNode{Name: "Review", Caption: "Review",
			BoundaryEvents: []ast.WorkflowBoundaryEventNode{{EventType: eventType, Delay: "addDays([%CurrentDateTime%], 1)"}}}
	}
	inSplit := func(a ast.WorkflowActivityNode) ast.WorkflowActivityNode {
		return &ast.WorkflowParallelSplitNode{Paths: []ast.WorkflowParallelPathNode{{Activities: []ast.WorkflowActivityNode{a}}}}
	}
	for _, c := range []struct {
		name         string
		act          ast.WorkflowActivityNode
		below, floor [3]int
		want         string
	}{
		{"wait for notification", &ast.WorkflowWaitForNotificationNode{Name: "w"}, [3]int{9, 24, 30}, [3]int{10, 0, 0}, "wait for notification"},
		{"wait for timer", &ast.WorkflowWaitForTimerNode{Name: "w", DelayExpression: "addHours([%CurrentDateTime%], 1)"}, [3]int{10, 5, 0}, [3]int{10, 6, 0}, "wait for timer"},
		{"nested wait for timer", inSplit(&ast.WorkflowWaitForTimerNode{Name: "w"}), [3]int{10, 5, 0}, [3]int{10, 6, 0}, "wait for timer"},
		{"interrupting timer", onTask("InterruptingTimer"), [3]int{10, 19, 0}, [3]int{10, 20, 0}, "interrupting timer"},
		{"non interrupting timer", onTask("NonInterruptingTimer"), [3]int{10, 19, 0}, [3]int{10, 20, 0}, "interrupting timer"},
		{"bare timer", onTask("Timer"), [3]int{10, 13, 0}, [3]int{10, 14, 0}, "boundary event timer"},
		{"timer on a wait for notification", &ast.WorkflowWaitForNotificationNode{Name: "w",
			BoundaryEvents: []ast.WorkflowBoundaryEventNode{{EventType: "InterruptingTimer"}}}, [3]int{10, 19, 0}, [3]int{10, 20, 0}, "interrupting timer"},
	} {
		for _, stmt := range []ast.Statement{
			&ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{c.act}},
			&ast.AlterWorkflowStmt{Operations: []ast.AlterWorkflowOp{
				&ast.InsertAfterOp{NewActivities: []ast.WorkflowActivityNode{c.act}}}},
		} {
			prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, stmt}}
			vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
			if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want) || vs[0].Suggestion == "" {
				t.Errorf("%s (%T) below its floor: want one MDL-VERSION01 naming %q with a hint, got %+v", c.name, stmt, c.want, vs)
			}
			if vs := versionGateViolations(versionCtx(t, c.floor[0], c.floor[1], c.floor[2]), prog); len(vs) != 0 {
				t.Errorf("%s (%T): reported at its floor %v: %+v", c.name, stmt, c.floor, vs)
			}
		}
	}

	// A boundary event `alter workflow` inserts is the op's own, not an
	// activity's; an empty kind is an interrupting timer.
	for _, eventType := range []string{"", "InterruptingTimer", "NonInterruptingTimer"} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
			&ast.AlterWorkflowStmt{Operations: []ast.AlterWorkflowOp{&ast.InsertBoundaryEventOp{ActivityRef: "Review", EventType: eventType}}}}}
		vs := versionGateViolations(versionCtx(t, 10, 19, 0), prog)
		if len(vs) != 1 || !strings.Contains(vs[0].Message, "interrupting timer") {
			t.Errorf("insert boundary event %q on 10.19: %+v", eventType, vs)
		}
		if vs := versionGateViolations(versionCtx(t, 10, 20, 0), prog); len(vs) != 0 {
			t.Errorf("insert boundary event %q on 10.20: %+v", eventType, vs)
		}
	}
}

// A lock or unlock naming its workflow stores a 10.0 selection type; one
// naming none (`lock workflow all`) is refused elsewhere, and is not this gate.
func TestCheckVersionGates_WorkflowDefinitionSelection(t *testing.T) {
	for _, st := range []ast.MicroflowStatement{
		&ast.LockWorkflowStmt{Workflow: "M.WF"},
		&ast.LockWorkflowStmt{WorkflowVariable: "WF", PauseAllWorkflows: true},
		&ast.UnlockWorkflowStmt{Workflow: "M.WF"},
		&ast.IfStmt{ThenBody: []ast.MicroflowStatement{&ast.UnlockWorkflowStmt{WorkflowVariable: "WF"}}},
	} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
			&ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{st}}}}
		vs := versionGateViolations(versionCtx(t, 9, 24, 30), prog)
		if len(vs) != 1 || !strings.Contains(vs[0].Message, "lock workflow / unlock workflow on a workflow") || vs[0].Suggestion == "" {
			t.Errorf("%T on 9.24: %+v", st, vs)
		}
		if vs := versionGateViolations(versionCtx(t, 10, 0, 0), prog); len(vs) != 0 {
			t.Errorf("%T on 10.0: %+v", st, vs)
		}
	}
	bare := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{&ast.LockWorkflowStmt{PauseAllWorkflows: true}}}}}
	if vs := versionGateViolations(versionCtx(t, 9, 24, 30), bare); len(vs) != 0 {
		t.Errorf("a lock naming no workflow reported by the selection gate: %+v", vs)
	}
}
