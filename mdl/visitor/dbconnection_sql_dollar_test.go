// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestDatabaseQuery_DollarSQLWithParameterDefault covers a silent data-loss bug:
// a query whose SQL is a $$…$$ dollar string AND whose parameter carries a
// DEFAULT '…' lost the SQL entirely — q.SQL was set from STRING_LITERAL(0),
// which in that combination is the parameter's default, not the query.
//
// The result stored a connection whose query body was the string "sales", and
// `mx check` reported 0 errors either way, so nothing surfaced it. The
// parameter-index logic further down the same function already compensates for
// the dollar-string case; the SQL extraction did not.
func TestDatabaseQuery_DollarSQLWithParameterDefault(t *testing.T) {
	const wantSQL = "SELECT id FROM t WHERE d = {d}"

	cases := map[string]struct {
		src         string
		wantDefault string // "" when the case writes no DEFAULT clause
	}{
		"dollar SQL + parameter default": {wantDefault: "sales", src: `create database connection M.C
  type 'BYOD' connection string @M.U username @M.N password @M.P
begin
  query Q sql $$` + wantSQL + `$$ parameter d: String default 'sales' returns M.E map ( id as Id );
end;`},
		// The forms that already worked must keep working.
		"dollar SQL, no default": {src: `create database connection M.C
  type 'BYOD' connection string @M.U username @M.N password @M.P
begin
  query Q sql $$` + wantSQL + `$$ parameter d: String returns M.E map ( id as Id );
end;`},
		"quoted SQL + parameter default": {wantDefault: "sales", src: `create database connection M.C
  type 'BYOD' connection string @M.U username @M.N password @M.P
begin
  query Q sql 'SELECT id FROM t WHERE d = {d}' parameter d: String default 'sales' returns M.E map ( id as Id );
end;`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			stmt, ok := prog.Statements[0].(*ast.CreateDatabaseConnectionStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.CreateDatabaseConnectionStmt", prog.Statements[0])
			}
			if len(stmt.Queries) != 1 {
				t.Fatalf("got %d queries, want 1", len(stmt.Queries))
			}
			q := stmt.Queries[0]
			if q.SQL != wantSQL {
				t.Errorf("SQL = %q, want %q — the query body was replaced", q.SQL, wantSQL)
			}
			if len(q.Parameters) != 1 {
				t.Fatalf("got %d parameters, want 1", len(q.Parameters))
			}
			if got := q.Parameters[0].DefaultValue; got != tc.wantDefault {
				t.Errorf("parameter default = %q, want %q", got, tc.wantDefault)
			}
		})
	}
}
