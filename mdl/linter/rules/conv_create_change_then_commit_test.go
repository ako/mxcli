// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// CONV021: a Create or Change object activity without commit, directly followed
// by a Commit of the same object, is one create/change with Commit set.

func activityWithID(id string, a microflows.MicroflowAction) *microflows.ActionActivity {
	aa := &microflows.ActionActivity{Action: a}
	aa.ID = model.ID(id)
	return aa
}

func flow(from, to string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{OriginID: model.ID(from), DestinationID: model.ID(to)}
}

func createOrder(commit microflows.CommitType) *microflows.ActionActivity {
	return activityWithID("create", &microflows.CreateObjectAction{
		EntityQualifiedName: "Shop.Order",
		OutputVariable:      "Order",
		Commit:              commit,
	})
}

func changeOrder(commit microflows.CommitType) *microflows.ActionActivity {
	return activityWithID("change", &microflows.ChangeObjectAction{ChangeVariable: "Order", Commit: commit})
}

func commitVar(id, variable string, withEvents bool) *microflows.ActionActivity {
	return activityWithID(id, &microflows.CommitObjectsAction{CommitVariable: variable, WithEvents: withEvents})
}

func lintPairs(objects []microflows.MicroflowObject, flows ...*microflows.SequenceFlow) []linter.Violation {
	oc := &microflows.MicroflowObjectCollection{Objects: objects, Flows: flows}
	return findCreateChangeThenCommit(oc, testMicroflow(), NewCreateChangeThenCommitRule())
}

func TestCreateThenCommit_IsReported(t *testing.T) {
	got := lintPairs(
		[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), commitVar("commit", "Order", true)},
		flow("create", "commit"),
	)
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.RuleID != "CONV021" || v.Severity != linter.SeverityWarning {
		t.Errorf("got %s/%s, want CONV021/warning", v.RuleID, v.Severity)
	}
	if !strings.Contains(v.Message, "creates $Order") || !strings.Contains(v.Message, "Create object activity") {
		t.Errorf("message %q should name the create and the variable", v.Message)
	}
	if !strings.Contains(v.Suggestion, "Commit to Yes on the Create object activity") ||
		!strings.Contains(v.Suggestion, "$Order = create …(…) commit;") {
		t.Errorf("suggestion %q should name Commit = Yes and the MDL form", v.Suggestion)
	}
}

// A commit without events maps onto Yes without events, and Refresh in client
// set on either activity is kept.
func TestChangeThenCommitWithoutEvents_KeepsTheSettings(t *testing.T) {
	commit := commitVar("commit", "Order", false)
	commit.Action.(*microflows.CommitObjectsAction).RefreshInClient = true
	got := lintPairs(
		[]microflows.MicroflowObject{changeOrder(microflows.CommitTypeNo), commit},
		flow("change", "commit"),
	)
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d: %+v", len(got), got)
	}
	s := got[0].Suggestion
	if !strings.Contains(s, "Yes without events, with Refresh in client on the Change object activity") ||
		!strings.Contains(s, "change $Order (…) commit without events refresh;") {
		t.Errorf("suggestion %q should carry without events and refresh", s)
	}
	if !strings.Contains(got[0].Message, "changes $Order") {
		t.Errorf("message %q should name the change", got[0].Message)
	}
}

// A pair inside a loop body is found there; the loop body has its own flows.
func TestCreateThenCommit_InsideALoop(t *testing.T) {
	loop := &microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{changeOrder(microflows.CommitTypeNo), commitVar("commit", "Order", true)},
		Flows:   []*microflows.SequenceFlow{flow("change", "commit")},
	}}
	if got := lintPairs([]microflows.MicroflowObject{loop}); len(got) != 1 {
		t.Fatalf("want 1 violation from the loop body, got %d", len(got))
	}
}

// Variable names are matched as Mendix does, without regard to case.
func TestCreateThenCommit_VariableCaseInsensitive(t *testing.T) {
	got := lintPairs(
		[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), commitVar("commit", "order", true)},
		flow("create", "commit"),
	)
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d", len(got))
	}
}

// Each of these is not the pattern, or merging would change what the flow
// does, so none is reported.
func TestCreateThenCommit_NotReported(t *testing.T) {
	customCommit := commitVar("commit", "Order", true)
	customCommit.Action.(*microflows.CommitObjectsAction).ErrorHandlingType = microflows.ErrorHandlingTypeCustom
	customCreate := createOrder(microflows.CommitTypeNo)
	customCreate.Action.(*microflows.CreateObjectAction).ErrorHandlingType = microflows.ErrorHandlingTypeCustomWithoutRollback
	errorFlow := flow("create", "commit")
	errorFlow.IsErrorHandler = true
	log := activityWithID("log", &microflows.LogMessageAction{})

	cases := []struct {
		name    string
		objects []microflows.MicroflowObject
		flows   []*microflows.SequenceFlow
	}{
		{"commit of another object",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), commitVar("commit", "Line", true)},
			[]*microflows.SequenceFlow{flow("create", "commit")}},
		{"create already commits",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeYes), commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{flow("create", "commit")}},
		{"commit setting unknown",
			[]microflows.MicroflowObject{createOrder(""), commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{flow("create", "commit")}},
		{"an activity in between",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), log, commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{flow("create", "log"), flow("log", "commit")}},
		{"commit reached by another path too",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), log, commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{flow("create", "commit"), flow("log", "commit")}},
		{"reached through the error handler flow",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{errorFlow}},
		{"commit has custom error handling",
			[]microflows.MicroflowObject{createOrder(microflows.CommitTypeNo), customCommit},
			[]*microflows.SequenceFlow{flow("create", "commit")}},
		{"create has custom error handling",
			[]microflows.MicroflowObject{customCreate, commitVar("commit", "Order", true)},
			[]*microflows.SequenceFlow{flow("create", "commit")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lintPairs(c.objects, c.flows...); len(got) != 0 {
				t.Errorf("want no violation, got %+v", got)
			}
		})
	}
}
