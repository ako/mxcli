// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestIdempotencyGuardsParse covers the guard spellings added for sudoku
// finding #10, which are what make a domain script re-runnable.
func TestIdempotencyGuardsParse(t *testing.T) {
	prog, errs := Build(`
create entity if not exists M."Game" ("Level": string(20));
create non-persistent entity if not exists M."Draft" ("Note": string(50));
create association if not exists M.Move_Game from M.Move to M.Game;
alter entity M."Game"
  add attribute if not exists "Score": integer,
  add index if not exists on ("Level"),
  drop index if exists ("Score");
`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %s", errsText(errs))
	}
	if len(prog.Statements) != 6 {
		t.Fatalf("got %d statements, want 6", len(prog.Statements))
	}

	ent, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok || !ent.IfNotExists {
		t.Errorf("create entity: IfNotExists not carried (%T)", prog.Statements[0])
	}
	if ent.Name.String() != "M.Game" {
		t.Errorf("the guard swallowed the name: got %q", ent.Name.String())
	}
	if np, ok := prog.Statements[1].(*ast.CreateEntityStmt); !ok || !np.IfNotExists || np.Kind != ast.EntityNonPersistent {
		t.Errorf("non-persistent entity: guard or kind lost (%+v)", prog.Statements[1])
	}
	if assoc, ok := prog.Statements[2].(*ast.CreateAssociationStmt); !ok || !assoc.IfNotExists {
		t.Errorf("create association: IfNotExists not carried (%T)", prog.Statements[2])
	}

	add, ok := prog.Statements[4].(*ast.AlterEntityStmt)
	if !ok || add.Operation != ast.AlterEntityAddIndex || !add.IfNotExists {
		t.Fatalf("add index: guard not carried (%+v)", prog.Statements[4])
	}
	if add.Index == nil || len(add.Index.Columns) != 1 || add.Index.Columns[0].Name != "Level" {
		t.Errorf("add index: columns lost (%+v)", add.Index)
	}

	drop, ok := prog.Statements[5].(*ast.AlterEntityStmt)
	if !ok || drop.Operation != ast.AlterEntityDropIndex || !drop.IfExists {
		t.Fatalf("drop index: guard not carried (%+v)", prog.Statements[5])
	}
	if drop.Index == nil || len(drop.Index.Columns) != 1 || drop.Index.Columns[0].Name != "Score" {
		t.Errorf("drop index: columns lost (%+v)", drop.Index)
	}
	if drop.IndexName != "" {
		t.Errorf("the column form must not also set IndexName, got %q", drop.IndexName)
	}
}

// TestUnguardedFormsAreUnchanged is the control: the guard is optional, and
// adding it to the grammar must not have made it implicit.
func TestUnguardedFormsAreUnchanged(t *testing.T) {
	prog, errs := Build(`
create entity M."Game" ("Level": string(20));
alter entity M."Game" add index on ("Level"), drop index idx1;
`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %s", errsText(errs))
	}
	if ent := prog.Statements[0].(*ast.CreateEntityStmt); ent.IfNotExists {
		t.Error("create entity picked up a guard nobody wrote")
	}
	if add := prog.Statements[1].(*ast.AlterEntityStmt); add.IfNotExists {
		t.Error("add index picked up a guard nobody wrote")
	}
	drop := prog.Statements[2].(*ast.AlterEntityStmt)
	if drop.IfExists {
		t.Error("drop index picked up a guard nobody wrote")
	}
	if drop.IndexName != "idx1" || drop.Index != nil {
		t.Errorf("the ordinal drop form must still reach the executor as a name, got %+v", drop)
	}
}
