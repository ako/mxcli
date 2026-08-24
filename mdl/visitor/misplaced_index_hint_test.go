// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

const indexHint = "An INDEX belongs AFTER the attribute parentheses"

// TestMisplacedIndexHint covers sudoku finding #4, which reported that
// `index … on (…)` "does not parse" inside `create entity`. The syntax is not
// missing — it belongs after the closing parenthesis — but ANTLR reports the
// index name as an attribute missing its type, so the error reads as though
// indexes were unsupported there.
func TestMisplacedIndexHint(t *testing.T) {
	src := `create entity M."Cell" (
  "Row": integer,
  "Col": integer,
  index "IdxRowCol" on ("Row", "Col")
);`
	_, errs := Build(src)
	if len(errs) == 0 {
		t.Fatal("expected syntax errors for an index inside the attribute list")
	}
	joined := errsText(errs)
	if !strings.Contains(joined, indexHint) {
		t.Errorf("no misplaced-index hint in:\n%s", joined)
	}
	// The old behaviour: the apostrophe heuristic fired on the short leftover
	// tokens and sent the author looking for a quoting problem.
	if strings.Contains(joined, "unescaped apostrophe") {
		t.Errorf("apostrophe hint still fires on a misplaced index:\n%s", joined)
	}
}

// TestMisplacedIndexHintIsNotRepeated guards the cascade. One misplaced index
// yields four ANTLR errors on the same line; repeating a six-line worked example
// under each buries the errors it explains.
func TestMisplacedIndexHintIsNotRepeated(t *testing.T) {
	src := `create entity M."Cell" (
  "Row": integer,
  index "IdxRowCol" on ("Row", "Col")
);`
	_, errs := Build(src)
	if n := strings.Count(errsText(errs), indexHint); n != 1 {
		t.Errorf("hint appears %d times, want exactly 1:\n%s", n, errsText(errs))
	}
	// The control: suppressing the repeat must not suppress the errors.
	if len(errs) < 2 {
		t.Errorf("expected the cascade to still report every error, got %d", len(errs))
	}
}

// TestValidIndexFormsStillParse is the false-positive control. All three
// supported spellings must remain clean — in particular the standalone
// `create index Idx on Mod.Entity (Col)`, which starts with the same token and
// is told apart only by naming the entity between ON and the parenthesis.
func TestValidIndexFormsStillParse(t *testing.T) {
	for _, src := range []string{
		`create entity M."Cell" ("Row": integer, "Col": integer) index "IdxRowCol" on ("Row", "Col");`,
		`create entity M."Cell" ("Row": integer) index ("Row");`,
		`alter entity M."Cell" add index "IdxRow" on ("Row");`,
		`create index IdxCol on M.Cell ("Col");`,
	} {
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("%s\n  unexpected errors: %s", src, errsText(errs))
		}
	}
}

// TestStandaloneCreateIndexProducesAStatement guards the silent no-op found
// while fixing #4: `createIndexStatement` had been in the grammar with no
// listener behind it, so the statement parsed to nothing and `exec` reported
// nothing and did nothing — even for an entity that does not exist.
func TestStandaloneCreateIndexProducesAStatement(t *testing.T) {
	prog, errs := Build(`create index IdxRowCol on M.Cell ("Row", "Col" desc);`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %s", errsText(errs))
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1 — a parse that yields nothing is a silent no-op", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.AlterEntityStmt", prog.Statements[0])
	}
	if stmt.Operation != ast.AlterEntityAddIndex {
		t.Errorf("operation = %v, want AlterEntityAddIndex", stmt.Operation)
	}
	if stmt.Name.String() != "M.Cell" {
		t.Errorf("entity = %q, want M.Cell", stmt.Name)
	}
	if stmt.Index == nil || len(stmt.Index.Columns) != 2 {
		t.Fatalf("index = %+v, want 2 columns", stmt.Index)
	}
	if stmt.Index.Columns[0].Name != "Row" || stmt.Index.Columns[0].Descending {
		t.Errorf("column 0 = %+v, want Row ascending", stmt.Index.Columns[0])
	}
	if stmt.Index.Columns[1].Name != "Col" || !stmt.Index.Columns[1].Descending {
		t.Errorf("column 1 = %+v, want Col descending", stmt.Index.Columns[1])
	}
}

func errsText(errs []error) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}
