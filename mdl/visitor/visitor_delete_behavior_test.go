// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #901. `DELETE_BEHAVIOR PREVENT` parsed, reported "Modified
// association", and stored DeleteMeButKeepReferences — overwriting whatever the
// association had before, which for the reporter was DELETE_CASCADE.
//
// The grammar and the generated parser were never at fault: DeleteBehaviorContext
// exposes an accessor for all five tokens (mdl_parser.go, DELETE_AND_REFERENCES /
// DELETE_BUT_KEEP_REFERENCES / DELETE_IF_NO_REFERENCES / CASCADE / PREVENT).
// buildDeleteBehavior called CASCADE() and nothing else, so the other four fell
// through to the zero value, ast.DeleteKeepReferences — a legal behaviour, which
// is why nothing downstream could tell it had been substituted.
//
// The table is exhaustive on purpose. A test for PREVENT alone passes against a
// fix that maps PREVENT and leaves DELETE_AND_REFERENCES — the canonical spelling
// of cascade, whose alias CASCADE works — still silently downgraded. The intended
// mapping is not invented here; it is the one `mxcli syntax
// domain-model.association.delete-behavior` has always printed.
func TestBuildDeleteBehavior_EverySpelling(t *testing.T) {
	cases := []struct {
		token string
		want  ast.DeleteBehavior
	}{
		{"DELETE_BUT_KEEP_REFERENCES", ast.DeleteKeepReferences},
		{"DELETE_AND_REFERENCES", ast.DeleteCascade},
		{"CASCADE", ast.DeleteCascade},
		{"DELETE_IF_NO_REFERENCES", ast.DeleteIfNoReferences},
		{"PREVENT", ast.DeleteIfNoReferences},
	}

	for _, tc := range cases {
		t.Run("create/"+tc.token, func(t *testing.T) {
			stmt := parseCreateAssoc(t, `CREATE ASSOCIATION M.C_P FROM M.C TO M.P TYPE Reference DELETE_BEHAVIOR `+tc.token+`;`)
			if stmt.DeleteBehavior != tc.want {
				t.Errorf("DELETE_BEHAVIOR %s = %v, want %v", tc.token, stmt.DeleteBehavior, tc.want)
			}
		})

		t.Run("alter/"+tc.token, func(t *testing.T) {
			stmt := parseAlterAssoc(t, `ALTER ASSOCIATION M.C_P SET DELETE_BEHAVIOR `+tc.token+`;`)
			if stmt.Operation != ast.AlterAssociationSetDeleteBehavior {
				t.Fatalf("operation = %v, want AlterAssociationSetDeleteBehavior", stmt.Operation)
			}
			if stmt.DeleteBehavior != tc.want {
				t.Errorf("SET DELETE_BEHAVIOR %s = %v, want %v", tc.token, stmt.DeleteBehavior, tc.want)
			}
		})
	}
}

// The control for the table above: an association that says nothing about delete
// behaviour must still land on Mendix's default. Without this, "map every token"
// could be satisfied by a change that broke the unspecified case.
func TestBuildDeleteBehavior_OmittedIsKeepReferences(t *testing.T) {
	stmt := parseCreateAssoc(t, `CREATE ASSOCIATION M.C_P FROM M.C TO M.P TYPE Reference;`)
	if stmt.DeleteBehavior != ast.DeleteKeepReferences {
		t.Errorf("omitted DELETE_BEHAVIOR = %v, want DeleteKeepReferences", stmt.DeleteBehavior)
	}
}

func parseCreateAssoc(t *testing.T, src string) *ast.CreateAssociationStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateAssociationStmt", prog.Statements[0])
	}
	return stmt
}

func parseAlterAssoc(t *testing.T, src string) *ast.AlterAssociationStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterAssociationStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.AlterAssociationStmt", prog.Statements[0])
	}
	return stmt
}
