// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#586: a CREATE WORKFLOW clause written in the "wrong" position was a
// parse error — the grammar was a fixed sequence of optional clauses, so each
// clause was optional but its position was not. The reported symptom is a token
// error naming neither the clause nor the rule:
//
//	line 5:4 mismatched input 'ON' expecting ';'
//
// These tests hold the two properties the fix has to have: a shuffled clause
// order parses, and it produces the SAME AST as the canonical order. Both
// halves matter — a grammar that merely accepts the tokens while the visitor
// still reads them positionally would mis-assign every qualified name.

func TestWorkflowUserTaskClauseOrderIsFree(t *testing.T) {
	canonical := `CREATE WORKFLOW M.WF
BEGIN
  USER TASK ut1 'Review'
    PAGE M.TaskPage
    TARGETING GROUPS XPATH '[Name = ''Admin'']'
    ON CREATED MICROFLOW M.OnCreated
    ENTITY M.Order
    DUE DATE '${PT4H}'
    DESCRIPTION 'Review the order'
    OUTCOMES
      'Approve' { }
      'Reject' { }
    BOUNDARY EVENT INTERRUPTING TIMER '${PT1H}';
END WORKFLOW;`

	// The order the reporter reached for: on-created first, page later.
	shuffled := `CREATE WORKFLOW M.WF
BEGIN
  USER TASK ut1 'Review'
    ON CREATED MICROFLOW M.OnCreated
    DESCRIPTION 'Review the order'
    BOUNDARY EVENT INTERRUPTING TIMER '${PT1H}'
    ENTITY M.Order
    OUTCOMES
      'Approve' { }
      'Reject' { }
    TARGETING GROUPS XPATH '[Name = ''Admin'']'
    DUE DATE '${PT4H}'
    PAGE M.TaskPage;
END WORKFLOW;`

	want := buildWorkflowStmt(t, canonical)
	got := buildWorkflowStmt(t, shuffled)

	if !reflect.DeepEqual(got.Activities, want.Activities) {
		t.Errorf("shuffled clause order built a different user task\n got: %#v\nwant: %#v",
			got.Activities[0], want.Activities[0])
	}

	ut, ok := want.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("expected *ast.WorkflowUserTaskNode, got %T", want.Activities[0])
	}
	// Spot-check the clauses that the positional reader used to assign by
	// counting qualified names, so a DeepEqual of two equally-wrong trees
	// cannot pass this test.
	if ut.Page.String() != "M.TaskPage" {
		t.Errorf("Page = %q, want M.TaskPage", ut.Page.String())
	}
	if ut.OnCreated.String() != "M.OnCreated" {
		t.Errorf("OnCreated = %q, want M.OnCreated", ut.OnCreated.String())
	}
	if ut.Entity.String() != "M.Order" {
		t.Errorf("Entity = %q, want M.Order", ut.Entity.String())
	}
	if ut.Targeting.Kind != "group_xpath" || ut.Targeting.XPath != "[Name = 'Admin']" {
		t.Errorf("Targeting = %q %q, want group_xpath [Name = 'Admin']", ut.Targeting.Kind, ut.Targeting.XPath)
	}
	if ut.DueDate != "${PT4H}" {
		t.Errorf("DueDate = %q", ut.DueDate)
	}
	if ut.TaskDescription != "Review the order" {
		t.Errorf("TaskDescription = %q", ut.TaskDescription)
	}
	if ut.Caption != "Review" {
		t.Errorf("Caption = %q", ut.Caption)
	}
}

func TestWorkflowMultiUserTaskClauseOrderIsFree(t *testing.T) {
	canonical := `CREATE WORKFLOW M.WF
BEGIN
  MULTI USER TASK ut1 'Review'
    PAGE M.TaskPage
    TARGETING MICROFLOW M.PickUsers
    ON CREATED MICROFLOW M.OnCreated
    ENTITY M.Order
    DUE DATE '${PT4H}'
    DESCRIPTION 'Review the order'
    PARTICIPANTS 60 PERCENT
    DECIDE BY MAJORITY MORE THAN HALF FALLBACK 'Reject'
    AWAIT ALL USERS
    OUTCOMES
      'Approve' { }
      'Reject' { };
END WORKFLOW;`

	shuffled := `CREATE WORKFLOW M.WF
BEGIN
  MULTI USER TASK ut1 'Review'
    AWAIT ALL USERS
    OUTCOMES
      'Approve' { }
      'Reject' { }
    ON CREATED MICROFLOW M.OnCreated
    DECIDE BY MAJORITY MORE THAN HALF FALLBACK 'Reject'
    DESCRIPTION 'Review the order'
    PAGE M.TaskPage
    PARTICIPANTS 60 PERCENT
    ENTITY M.Order
    TARGETING MICROFLOW M.PickUsers
    DUE DATE '${PT4H}';
END WORKFLOW;`

	want := buildWorkflowStmt(t, canonical)
	got := buildWorkflowStmt(t, shuffled)

	if !reflect.DeepEqual(got.Activities, want.Activities) {
		t.Errorf("shuffled clause order built a different multi user task\n got: %#v\nwant: %#v",
			got.Activities[0], want.Activities[0])
	}

	ut, ok := want.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("expected *ast.WorkflowUserTaskNode, got %T", want.Activities[0])
	}
	if !ut.IsMultiUser || !ut.AwaitAllUsers {
		t.Errorf("IsMultiUser=%v AwaitAllUsers=%v, want both true", ut.IsMultiUser, ut.AwaitAllUsers)
	}
	if ut.OnCreated.String() != "M.OnCreated" {
		t.Errorf("OnCreated = %q, want M.OnCreated", ut.OnCreated.String())
	}
	if ut.Entity.String() != "M.Order" {
		t.Errorf("Entity = %q, want M.Order", ut.Entity.String())
	}
}

func TestWorkflowHeaderClauseOrderIsFree(t *testing.T) {
	canonical := `CREATE WORKFLOW M.WF
  FOLDER 'Flows'
  PARAMETER $Order: M.Order
  DISPLAY 'Order review'
  DESCRIPTION 'Reviews an order'
  EXPORT LEVEL Hidden
  OVERVIEW PAGE M.Overview
  DUE DATE '${P1D}'
  ON ANY WORKFLOW EVENT MICROFLOW M.OnEvent AS 'all'
BEGIN
  ANNOTATION 'body';
END WORKFLOW;`

	// The reporter's note gives the order they worked out empirically; this is
	// a different one, which must mean the same thing.
	shuffled := `CREATE WORKFLOW M.WF
  ON ANY WORKFLOW EVENT MICROFLOW M.OnEvent AS 'all'
  DUE DATE '${P1D}'
  DESCRIPTION 'Reviews an order'
  OVERVIEW PAGE M.Overview
  DISPLAY 'Order review'
  PARAMETER $Order: M.Order
  EXPORT LEVEL Hidden
  FOLDER 'Flows'
BEGIN
  ANNOTATION 'body';
END WORKFLOW;`

	want := buildWorkflowStmt(t, canonical)
	got := buildWorkflowStmt(t, shuffled)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("shuffled header order built a different workflow\n got: %#v\nwant: %#v", got, want)
	}

	if want.Folder != "Flows" {
		t.Errorf("Folder = %q", want.Folder)
	}
	if want.ParameterVar != "$Order" || want.ParameterEntity.String() != "M.Order" {
		t.Errorf("Parameter = %q %q", want.ParameterVar, want.ParameterEntity.String())
	}
	if want.DisplayName != "Order review" {
		t.Errorf("DisplayName = %q", want.DisplayName)
	}
	if want.Description != "Reviews an order" {
		t.Errorf("Description = %q", want.Description)
	}
	if want.ExportLevel != "Hidden" {
		t.Errorf("ExportLevel = %q", want.ExportLevel)
	}
	if want.OverviewPage.String() != "M.Overview" {
		t.Errorf("OverviewPage = %q", want.OverviewPage.String())
	}
	if want.DueDate != "${P1D}" {
		t.Errorf("DueDate = %q", want.DueDate)
	}
	if len(want.EventHandlers) != 1 || !want.EventHandlers[0].AnyEvent {
		t.Errorf("EventHandlers = %#v", want.EventHandlers)
	}
}

// Order-freedom must not become "write it twice and the last one wins": the old
// grammar allowed each clause at most once, and a repeated-clause list silently
// overwriting is a worse failure than the parse error it replaces.
func TestWorkflowDuplicateClauseIsReported(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "user task page twice",
			input: `CREATE WORKFLOW M.WF
BEGIN
  USER TASK ut1 'Review'
    PAGE M.A
    PAGE M.B;
END WORKFLOW;`,
			want: "PAGE",
		},
		{
			name: "user task on created twice",
			input: `CREATE WORKFLOW M.WF
BEGIN
  USER TASK ut1 'Review'
    ON CREATED MICROFLOW M.A
    DESCRIPTION 'd'
    ON CREATED MICROFLOW M.B;
END WORKFLOW;`,
			want: "ON CREATED MICROFLOW",
		},
		{
			name: "header display twice",
			input: `CREATE WORKFLOW M.WF
  DISPLAY 'A'
  DISPLAY 'B'
BEGIN
END WORKFLOW;`,
			want: "DISPLAY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Build(tt.input)
			if len(errs) == 0 {
				t.Fatalf("expected a duplicate-clause error, got none")
			}
			joined := joinErrors(errs)
			if !strings.Contains(joined, tt.want) {
				t.Errorf("error does not name the duplicated clause %q:\n%s", tt.want, joined)
			}
			if !strings.Contains(strings.ToLower(joined), "at most once") {
				t.Errorf("error does not say the clause may appear at most once:\n%s", joined)
			}
		})
	}
}

func joinErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

// `targeting microflow` and `targeting xpath` fill the same slot — a user task
// stores one UserSource — so writing both is a duplicate, not two clauses. The
// sequence grammar accepted both and let the LAST one win, which is the
// order-dependence of #586 in its most damaging form.
func TestWorkflowTwoTargetingClausesAreADuplicate(t *testing.T) {
	input := `CREATE WORKFLOW M.WF
BEGIN
  USER TASK ut1 'Review'
    TARGETING MICROFLOW M.PickUsers
    TARGETING XPATH '[true()]';
END WORKFLOW;`

	_, errs := Build(input)
	if len(errs) == 0 {
		t.Fatal("expected a duplicate TARGETING error, got none")
	}
	if joined := joinErrors(errs); !strings.Contains(joined, "TARGETING") {
		t.Errorf("error does not name TARGETING:\n%s", joined)
	}
}

// Relaxing the clause ORDER must not relax the clause VOCABULARY: the
// multi-user-only clauses stay refused on a single user task.
func TestSingleUserTaskStillRefusesMultiUserClauses(t *testing.T) {
	for _, clause := range []string{
		"PARTICIPANTS 60 PERCENT",
		"DECIDE BY CONSENSUS",
		"AWAIT ALL USERS",
	} {
		t.Run(clause, func(t *testing.T) {
			input := "CREATE WORKFLOW M.WF\nBEGIN\n  USER TASK ut1 'Review'\n    " +
				clause + "\n    OUTCOMES 'Done' { };\nEND WORKFLOW;"
			if _, errs := Build(input); len(errs) == 0 {
				t.Errorf("%s was accepted on a single user task", clause)
			}
		})
	}
}
