// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func commitInLoopViolations(t *testing.T, body []ast.MicroflowStatement) []string {
	t.Helper()
	v := &microflowValidator{}
	v.checkCommitInLoop(body)
	var msgs []string
	for _, viol := range v.violations {
		if viol.RuleID == "MDL-PERF01" {
			msgs = append(msgs, viol.Message)
		}
	}
	return msgs
}

func commit(v string) ast.MicroflowStatement { return &ast.MfCommitStmt{Variable: v} }

func loopOver(list string, body ...ast.MicroflowStatement) ast.MicroflowStatement {
	return &ast.LoopStmt{ListVariable: list, LoopVariable: "Item", Body: body}
}

// The reported case: one commit per iteration, one database round trip per
// iteration. `lint` has known it as CONV011 all along, but only after the write
// and only project-wide (upstream mendixlabs/mxcli#1186).
func TestCommitInLoopIsReported(t *testing.T) {
	got := commitInLoopViolations(t, []ast.MicroflowStatement{
		loopOver("Items", commit("Item")),
	})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "CONV011") {
		t.Errorf("the message does not name CONV011, so a reader cannot tell this is "+
			"the same finding lint reports:\n%s", got[0])
	}
}

// CONTROL: a commit AFTER the loop is the fix this rule suggests, and must not
// be flagged — otherwise following the suggestion does not clear the warning.
func TestCommitAfterLoopIsNotReported(t *testing.T) {
	got := commitInLoopViolations(t, []ast.MicroflowStatement{
		loopOver("Items", &ast.ChangeObjectStmt{}),
		commit("Batch"),
	})
	if len(got) != 0 {
		t.Errorf("commit after the loop was flagged: %v", got)
	}
}

// A commit nested in a branch inside the loop still runs per iteration. Walking
// only the loop's direct statements would miss the realistic shape.
func TestCommitInsideABranchInsideALoopIsReported(t *testing.T) {
	got := commitInLoopViolations(t, []ast.MicroflowStatement{
		loopOver("Items", &ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{commit("Item")},
			ElseBody: []ast.MicroflowStatement{&ast.EnumSplitStmt{
				Cases:    []ast.EnumSplitCase{{Body: []ast.MicroflowStatement{commit("Other")}}},
				ElseBody: []ast.MicroflowStatement{commit("Third")},
			}},
		}),
	})
	if len(got) != 3 {
		t.Errorf("got %d findings, want 3 (if / enum case / enum else): %v", len(got), got)
	}
}

// THE ALIGNMENT CONTROL. A `while true` is built as an ExclusiveMerge back-edge,
// not a LoopedActivity, so CONV011 — which walks LoopedActivity — does not flag a
// commit inside one. Neither does this rule, deliberately.
//
// A commit there is arguably still N+1 at runtime, and that is exactly why the
// boundary is pinned: two rules for one concept that disagree on what counts is
// how a pair like this drifts. If the case is worth reporting it is worth
// reporting in both, and CONV011 is the one that sees the built flow.
func TestWhileTrueMatchesCONV011AndIsNotReported(t *testing.T) {
	whileTrue := &ast.WhileStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		Body:      []ast.MicroflowStatement{commit("Item")},
	}
	if got := commitInLoopViolations(t, []ast.MicroflowStatement{whileTrue}); len(got) != 0 {
		t.Errorf("a commit in `while true` was flagged, which CONV011 does not do: %v", got)
	}

	// CONTROL on the control: a while with a REAL condition IS a loop object, so
	// it must be flagged. Without this the exemption could be "never flag a
	// while", which would silently drop the whole construct.
	realWhile := &ast.WhileStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false},
		Body:      []ast.MicroflowStatement{commit("Item")},
	}
	if got := commitInLoopViolations(t, []ast.MicroflowStatement{realWhile}); len(got) != 1 {
		t.Errorf("a commit in a conditional while was not flagged: %v", got)
	}
}

// A loop with no commit is the ordinary case and must stay quiet.
func TestLoopWithoutCommitIsNotReported(t *testing.T) {
	if got := commitInLoopViolations(t, []ast.MicroflowStatement{
		loopOver("Items", &ast.ChangeObjectStmt{}),
	}); len(got) != 0 {
		t.Errorf("a loop with no commit was flagged: %v", got)
	}
}
