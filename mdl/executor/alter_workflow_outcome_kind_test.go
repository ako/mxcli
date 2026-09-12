// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#415. Three ALTER WORKFLOW ops each write one hard-coded outcome
// type into the target activity's Outcomes list:
//
//	INSERT OUTCOME    -> Workflows$UserTaskOutcome
//	INSERT PATH       -> Workflows$ParallelSplitOutcome
//	INSERT CONDITION  -> Workflows$*ConditionOutcome
//
// The lists are not interchangeable — the metamodel types them per activity
// (UserTask/MultiUserTask hold UserTaskOutcome, Decision/CallMicroflowTask hold
// ConditionOutcome, ParallelSplit holds ParallelSplitOutcome) — and none of the
// three checked what it was pointed at. Measured on mxbuild 11.10.0, every
// mismatch leaves a project that cannot be LOADED: `mx check` dies at "Loading
// the mpr file" with a .NET exception, before it validates anything, so Studio
// Pro will not open the project either.

// wfKindFixture returns a mock backend serving one stored workflow with a user
// task, a decision, a parallel split and a call microflow — one of every
// activity kind these ops can legitimately or illegitimately target.
func wfKindFixture(t *testing.T) (*ExecContext, *mock.MockBackend) {
	t.Helper()
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	wf.Flow = &workflows.Flow{
		Activities: []workflows.WorkflowActivity{
			&workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a1"}, Name: "task1"}},
			&workflows.ExclusiveSplitActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a2"}, Name: "decision9"}},
			&workflows.ParallelSplitActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a3"}, Name: "split1"}},
			&workflows.CallMicroflowTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a4"}, Name: "callMicroflow1"}},
		},
	}

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, mb
}

// alterWfRefErrors runs the guard both passes share.
func alterWfRefErrors(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if !ok {
		t.Fatalf("not an ALTER WORKFLOW statement: %T", prog.Statements[0])
	}
	return validateAlterWorkflowRefs(ctx, stmt, nil)
}

func TestAlterWorkflow_OutcomeOpOnWrongActivityKindIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		refused bool
	}{
		// INSERT OUTCOME writes a UserTaskOutcome.
		{"insert outcome on a decision", `alter workflow Sales.WF insert outcome 'X' on decision9 { };`, true},
		{"insert outcome on a parallel split", `alter workflow Sales.WF insert outcome 'X' on split1 { };`, true},
		{"insert outcome on a call microflow", `alter workflow Sales.WF insert outcome 'X' on callMicroflow1 { };`, true},
		{"insert outcome on a user task", `alter workflow Sales.WF insert outcome 'X' on task1 { };`, false},

		// INSERT PATH writes a ParallelSplitOutcome.
		{"insert path on a decision", `alter workflow Sales.WF insert path on decision9 { };`, true},
		{"insert path on a user task", `alter workflow Sales.WF insert path on task1 { };`, true},
		{"insert path on a parallel split", `alter workflow Sales.WF insert path on split1 { };`, false},

		// INSERT CONDITION writes a ConditionOutcome.
		{"insert condition on a user task", `alter workflow Sales.WF insert condition 'Sales.K.A' on task1 { };`, true},
		{"insert condition on a parallel split", `alter workflow Sales.WF insert condition 'Sales.K.A' on split1 { };`, true},
		{"insert condition on a decision", `alter workflow Sales.WF insert condition 'Sales.K.A' on decision9 { };`, false},
		{"insert condition on a call microflow", `alter workflow Sales.WF insert condition 'Sales.K.A' on callMicroflow1 { };`, false},

		// A boundary event is a separate list, and only four activity kinds
		// declare one. On a decision the write was silently dropped — mxcli
		// printed "Altered workflow" and the document was byte-identical.
		{"boundary event on a decision", `alter workflow Sales.WF insert boundary event on decision9 timer '1h' { };`, true},
		{"boundary event on a parallel split", `alter workflow Sales.WF insert boundary event on split1 timer '1h' { };`, true},
		{"boundary event on a user task", `alter workflow Sales.WF insert boundary event on task1 timer '1h' { };`, false},
		{"boundary event on a call microflow", `alter workflow Sales.WF insert boundary event on callMicroflow1 timer '1h' { };`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := wfKindFixture(t)
			errs := alterWfRefErrors(t, ctx, tc.src)
			got := false
			for _, e := range errs {
				if strings.Contains(e, "cannot hold") {
					got = true
				}
			}
			if got != tc.refused {
				t.Fatalf("refused = %v, want %v (errors: %v)", got, tc.refused, errs)
			}
		})
	}
}

// The message has to name the op that IS right for the target, or the author is
// told no without being told what to write instead.
func TestAlterWorkflow_RefusalNamesTheRightOp(t *testing.T) {
	ctx, _ := wfKindFixture(t)
	errs := alterWfRefErrors(t, ctx, `alter workflow Sales.WF insert outcome 'X' on decision9 { };`)
	if len(errs) == 0 {
		t.Fatal("expected a refusal")
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"INSERT CONDITION", "decision"} {
		if !strings.Contains(joined, want) {
			t.Errorf("refusal does not mention %q: %s", want, joined)
		}
	}
}

// An unresolvable or ambiguous reference must NOT be turned into a refusal by
// this guard — the mutator already reports those, with better information. A
// guard that refused on a failed lookup would reject valid scripts whose
// activity is addressed in a way only the stored document can settle.
func TestAlterWorkflow_UnknownActivityIsLeftToTheMutator(t *testing.T) {
	ctx, _ := wfKindFixture(t)
	errs := alterWfRefErrors(t, ctx, `alter workflow Sales.WF insert outcome 'X' on noSuchActivity { };`)
	for _, e := range errs {
		if strings.Contains(e, "cannot hold") {
			t.Fatalf("guard fired on an activity it could not resolve: %v", errs)
		}
	}
}

// The load-bearing assertion: exec must not reach the mutator. A refusal that
// still wrote would leave exactly the corrupt project the guard exists to
// prevent — and exec is reachable without `check` ever running.
func TestAlterWorkflow_RefusedOpNeverReachesTheMutator(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.ExclusiveSplitActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "a2"}, Name: "decision9"}},
	}}
	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)

	inserted := false
	mut := &mock.MockWorkflowMutator{
		InsertOutcomeFunc: func(string, int, string, []workflows.WorkflowActivity) error {
			inserted = true
			return nil
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListWorkflowsFunc:           func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		OpenWorkflowForMutationFunc: func(model.ID) (backend.WorkflowMutator, error) { return mut, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	prog, perrs := visitor.Build(`alter workflow Sales.WF insert outcome 'X' on decision9 { };`)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v", perrs)
	}
	err := execAlterWorkflow(ctx, prog.Statements[0].(*ast.AlterWorkflowStmt))
	if err == nil {
		t.Fatal("exec accepted an INSERT OUTCOME on a decision")
	}
	if inserted {
		t.Fatal("exec wrote the outcome anyway — the project would be unloadable")
	}
}
