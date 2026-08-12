// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestSQLAlias_KeywordNames covers the crash found by the syntax-registry
// example guard: `SQL DISCONNECT source` segfaulted `mxcli check`.
//
// `source` lexes as SOURCE_KW, so the rule's bare IDENTIFIER did not match.
// ANTLR error-recovered and still walked the tree, so the listener ran against a
// context whose IDENTIFIER() was nil and `.GetText()` panicked. Two things were
// wrong: the alias should accept a keyword (IMPORT FROM already did), and no
// listener should dereference a child without checking it.
//
// `source` is not a cherry-picked case — it is the alias in mxcli's own
// documented example for `mxcli syntax sql`.
func TestSQLAlias_KeywordNames(t *testing.T) {
	// Words that are MDL keywords and plausible connection/table names.
	for _, alias := range []string{"source", "table", "query", "view", "index", "key", "mydb"} {
		t.Run(alias, func(t *testing.T) {
			prog, errs := Build("SQL DISCONNECT " + alias + ";")
			if len(errs) > 0 {
				t.Fatalf("SQL DISCONNECT %s: %v", alias, errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("statements = %d, want 1", len(prog.Statements))
			}
			stmt, ok := prog.Statements[0].(*ast.SQLDisconnectStmt)
			if !ok {
				t.Fatalf("statement type = %T, want *ast.SQLDisconnectStmt", prog.Statements[0])
			}
			if stmt.Alias != alias {
				t.Errorf("Alias = %q, want %q", stmt.Alias, alias)
			}
		})
	}
}

// TestSQLStatements_KeywordAliasRoundTrip walks the whole SQL surface with a
// keyword alias, so a future rule that reverts to bare IDENTIFIER is caught on
// the statement it breaks rather than on DISCONNECT alone.
func TestSQLStatements_KeywordAliasRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		check func(t *testing.T, stmt ast.Statement)
	}{
		{
			name: "connect",
			src:  "SQL CONNECT postgres 'postgres://u:p@localhost:5432/db' AS source;",
			check: func(t *testing.T, stmt ast.Statement) {
				s, ok := stmt.(*ast.SQLConnectStmt)
				if !ok {
					t.Fatalf("type = %T", stmt)
				}
				if s.Alias != "source" || s.Driver != "postgres" {
					t.Errorf("got driver=%q alias=%q, want postgres/source", s.Driver, s.Alias)
				}
				if !strings.Contains(s.DSN, "localhost:5432") {
					t.Errorf("DSN = %q", s.DSN)
				}
			},
		},
		{
			name: "show tables",
			src:  "SQL source SHOW TABLES;",
			check: func(t *testing.T, stmt ast.Statement) {
				s, ok := stmt.(*ast.SQLShowTablesStmt)
				if !ok {
					t.Fatalf("type = %T", stmt)
				}
				if s.Alias != "source" {
					t.Errorf("Alias = %q, want source", s.Alias)
				}
			},
		},
		{
			name: "describe table",
			src:  "SQL source DESCRIBE users;",
			check: func(t *testing.T, stmt ast.Statement) {
				s, ok := stmt.(*ast.SQLDescribeTableStmt)
				if !ok {
					t.Fatalf("type = %T", stmt)
				}
				if s.Alias != "source" || s.Table != "users" {
					t.Errorf("got alias=%q table=%q, want source/users", s.Alias, s.Table)
				}
			},
		},
		{
			name: "query",
			src:  "SQL source SELECT * FROM users LIMIT 10;",
			check: func(t *testing.T, stmt ast.Statement) {
				s, ok := stmt.(*ast.SQLQueryStmt)
				if !ok {
					t.Fatalf("type = %T", stmt)
				}
				if s.Alias != "source" {
					t.Errorf("Alias = %q, want source", s.Alias)
				}
				if !strings.Contains(strings.ToUpper(s.Query), "SELECT") {
					t.Errorf("Query = %q", s.Query)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("Build(%q): %v", tt.src, errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("statements = %d, want 1", len(prog.Statements))
			}
			tt.check(t, prog.Statements[0])
		})
	}
}

// TestSQLGenerateConnector_KeywordAlias pins the index shift: the alias joined
// the identifierOrKeyword list, so the module moved from [0] to [1] and the
// table/view names from [1:] to [2:]. Getting that wrong would silently generate
// a connector into a module named after the connection.
func TestSQLGenerateConnector_KeywordAlias(t *testing.T) {
	prog, errs := Build("SQL source GENERATE CONNECTOR INTO MyModule TABLES (users, orders);")
	if len(errs) > 0 {
		t.Fatalf("Build: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("statements = %d, want 1", len(prog.Statements))
	}
	s, ok := prog.Statements[0].(*ast.SQLGenerateConnectorStmt)
	if !ok {
		t.Fatalf("type = %T", prog.Statements[0])
	}
	if s.Alias != "source" {
		t.Errorf("Alias = %q, want source", s.Alias)
	}
	if s.Module != "MyModule" {
		t.Errorf("Module = %q, want MyModule", s.Module)
	}
	if len(s.Tables) != 2 || s.Tables[0] != "users" || s.Tables[1] != "orders" {
		t.Errorf("Tables = %v, want [users orders]", s.Tables)
	}
}

// TestSQLDisconnect_MalformedDoesNotPanic is the robustness half. ANTLR keeps
// walking after a syntax error, so a listener must tolerate missing children —
// an unparseable statement is an error to report, never a crash.
func TestSQLDisconnect_MalformedDoesNotPanic(t *testing.T) {
	for _, src := range []string{
		"SQL DISCONNECT;",
		"SQL DISCONNECT 'quoted';",
		"SQL DISCONNECT 123;",
		"SQL CONNECT postgres AS;",
		"SQL DESCRIBE;",
	} {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Build(%q) panicked: %v", src, r)
				}
			}()
			_, _ = Build(src)
		})
	}
}
