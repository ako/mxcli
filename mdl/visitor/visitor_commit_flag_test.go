// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#779: MDL had no way to express the Commit flag on a create or
// change activity, and the builder hardcoded CommitTypeNo — so authoring or
// round-tripping a microflow through MDL silently cleared every commit flag in it.
// That is the object-orphaning failure mode the reporter hit.
//
// These tests cover the parse half: `COMMIT` / `COMMIT WITHOUT EVENTS` reach the AST,
// and the modifier is never confused with the standalone `COMMIT $Var` activity.
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func flowBody(t *testing.T, body string) []ast.MicroflowStatement {
	t.Helper()
	src := "create microflow M.F ($C: M.Contract)\nbegin\n  " + body + "\nend;"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", body, errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	return mf.Body
}

func TestCommitClause_Create(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ast.CommitFlag
	}{
		{"absent is No", "$O = create M.Order (Number = 'X');", ast.CommitNo},
		{"commit", "$O = create M.Order (Number = 'X') commit;", ast.CommitYes},
		{"commit without events", "$O = create M.Order (Number = 'X') commit without events;", ast.CommitYesWithoutEvents},
		{"no member list", "$O = create M.Order commit;", ast.CommitYes},
		// The error handler still follows the modifier.
		{"with on error", "$O = create M.Order (Number = 'X') commit on error rollback;", ast.CommitYes},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmts := flowBody(t, tc.body)
			c, ok := stmts[0].(*ast.CreateObjectStmt)
			if !ok {
				t.Fatalf("expected CreateObjectStmt, got %T", stmts[0])
			}
			if c.Commit != tc.want {
				t.Errorf("Commit = %v, want %v", c.Commit, tc.want)
			}
		})
	}
}

func TestCommitClause_Change(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    ast.CommitFlag
		refresh bool
	}{
		{"absent is No", "change $C (Status = 'P');", ast.CommitNo, false},
		{"commit", "change $C (Status = 'P') commit;", ast.CommitYes, false},
		{"commit without events", "change $C (Status = 'P') commit without events;", ast.CommitYesWithoutEvents, false},
		// Ordering: the commit modifier precedes refresh, and both must survive.
		{"commit then refresh", "change $C (Status = 'P') commit refresh;", ast.CommitYes, true},
		{"without events then refresh", "change $C (Status = 'P') commit without events refresh;", ast.CommitYesWithoutEvents, true},
		{"refresh alone still works", "change $C (Status = 'P') refresh;", ast.CommitNo, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmts := flowBody(t, tc.body)
			c, ok := stmts[0].(*ast.ChangeObjectStmt)
			if !ok {
				t.Fatalf("expected ChangeObjectStmt, got %T", stmts[0])
			}
			if c.Commit != tc.want {
				t.Errorf("Commit = %v, want %v", c.Commit, tc.want)
			}
			if c.RefreshInClient != tc.refresh {
				t.Errorf("RefreshInClient = %v, want %v", c.RefreshInClient, tc.refresh)
			}
		})
	}
}

// TestCommitClause_NotConfusedWithCommitActivity is the ambiguity guard. `COMMIT`
// both modifies a create/change and starts a standalone commit activity; the two
// must stay distinct. Mandatory statement semicolons are what make this decidable —
// without the terminator the modifier would greedily absorb the next statement.
func TestCommitClause_NotConfusedWithCommitActivity(t *testing.T) {
	stmts := flowBody(t, "$O = create M.Order (Number = 'X');\n  commit $O;")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	c, ok := stmts[0].(*ast.CreateObjectStmt)
	if !ok {
		t.Fatalf("statement 0 = %T, want *ast.CreateObjectStmt", stmts[0])
	}
	if c.Commit != ast.CommitNo {
		t.Errorf("the following commit activity was absorbed as a modifier: Commit = %v", c.Commit)
	}
	cm, ok := stmts[1].(*ast.MfCommitStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want *ast.MfCommitStmt", stmts[1])
	}
	if cm.Variable != "O" {
		t.Errorf("commit activity variable = %q, want %q", cm.Variable, "O")
	}
}

// A create carrying the modifier AND followed by a commit activity: both survive.
func TestCommitClause_ModifierAndActivityTogether(t *testing.T) {
	stmts := flowBody(t, "$O = create M.Order (Number = 'X') commit;\n  commit $C;")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if c := stmts[0].(*ast.CreateObjectStmt); c.Commit != ast.CommitYes {
		t.Errorf("create Commit = %v, want CommitYes", c.Commit)
	}
	if cm, ok := stmts[1].(*ast.MfCommitStmt); !ok || cm.Variable != "C" {
		t.Errorf("statement 1 = %T (%+v), want a commit activity on $C", stmts[1], stmts[1])
	}
}
