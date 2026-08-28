// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: completing an unassigned user task fails only at
// RUNTIME ("You can't complete this user task, it is not assigned to you"), the
// button appears to do nothing, and both checkers pass. About eight minutes of
// runtime archaeology.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func taskClaimWarnings(t *testing.T, body string) []string {
	t.Helper()
	src := "create microflow M.ACT ( $Task: System.WorkflowUserTask )\nbegin\n" + body + "\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, v := range ValidateTaskClaims(prog) {
		msgs = append(msgs, v.Message)
	}
	return msgs
}

func TestOutcomeWithoutAClaimIsWarned(t *testing.T) {
	got := taskClaimWarnings(t, "  set task outcome $Task 'Plan';")
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(got), got)
	}
	// The runtime message is the thing someone will search for once it bites.
	if !strings.Contains(got[0], "not assigned to you") {
		t.Errorf("warning does not quote the runtime failure:\n%s", got[0])
	}
}

func TestClaimBeforeOutcomeIsClean(t *testing.T) {
	// The control. This is the shape the report ended up with, and a rule that
	// flagged it would be telling people their working code is broken.
	got := taskClaimWarnings(t, `  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Plan';`)
	if len(got) != 0 {
		t.Errorf("correct claim-then-complete was warned: %v", got)
	}
}

func TestClaimAfterTheOutcomeStillWarns(t *testing.T) {
	// Order is the whole point — claiming afterwards does not make the completion
	// work, so collecting all claims first and then checking would pass this.
	got := taskClaimWarnings(t, `  set task outcome $Task 'Plan';
  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);`)
	if len(got) != 1 {
		t.Errorf("a claim written after the outcome was accepted: %v", got)
	}
}

func TestClaimOnADifferentTaskDoesNotCount(t *testing.T) {
	got := taskClaimWarnings(t, `  change $Other (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  set task outcome $Task 'Plan';`)
	if len(got) != 1 {
		t.Errorf("a claim on another variable satisfied the check: %v", got)
	}
}

func TestClaimInsideABranchCounts(t *testing.T) {
	// A warning must fail quiet. The claim is conditional here and this rule
	// cannot evaluate the condition, so it accepts it rather than reporting a
	// construct that may well be correct.
	got := taskClaimWarnings(t, `  if $Task != empty then
    change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  end if;
  set task outcome $Task 'Plan';`)
	if len(got) != 0 {
		t.Errorf("a claim inside a branch was not counted: %v", got)
	}
}

func TestAssigneesSpellingsAreAllRecognised(t *testing.T) {
	// The association parses qualified, bare and quoted, and all three reach the
	// model as the same write. Matching only one spelling would warn about code
	// that is already correct.
	for _, spelling := range []string{
		`System.WorkflowUserTask_Assignees`,
		`System."WorkflowUserTask_Assignees"`,
		`WorkflowUserTask_Assignees`,
	} {
		body := "  change $Task (" + spelling + " = [%CurrentUser%]);\n  set task outcome $Task 'Plan';"
		if got := taskClaimWarnings(t, body); len(got) != 0 {
			t.Errorf("spelling %s was not recognised as a claim: %v", spelling, got)
		}
	}
}
