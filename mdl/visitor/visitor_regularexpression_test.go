// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateRegularExpression(t *testing.T) {
	prog, errs := Build(`CREATE REGULAR EXPRESSION Val.Email (
		Expression: '^[^@]+@[^@]+$',
		Documentation: 'An email address'
	);`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateRegularExpressionStmt)
	if !ok {
		t.Fatalf("expected CreateRegularExpressionStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.String() != "Val.Email" {
		t.Errorf("Name = %s", stmt.Name.String())
	}
	if stmt.Expression != `^[^@]+@[^@]+$` {
		t.Errorf("Expression = %q", stmt.Expression)
	}
	if stmt.Documentation != "An email address" {
		t.Errorf("Documentation = %q", stmt.Documentation)
	}
}

// TestCreateRegularExpression_PatternsSurviveIntact is the point of the feature:
// a regex is full of characters that a naive unquoter would eat.
func TestCreateRegularExpression_PatternsSurviveIntact(t *testing.T) {
	tests := []struct{ name, literal, want string }{
		{"backslashes", `'\w+\.\d{2,}'`, `\w+\.\d{2,}`},
		{"doubled quote is one quote", `'^it''s$'`, `^it's$`},
		{"dotnet lookbehind", `'.*(?<!/)$'`, `.*(?<!/)$`},
		{"alternation and anchors", `'^[a-zA-Z0-9_-]+|$'`, `^[a-zA-Z0-9_-]+|$`},
		{"comma inside the pattern", `'^\d{2,4}$'`, `^\d{2,4}$`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(`CREATE REGULAR EXPRESSION Val.R ( Expression: ` + tt.literal + ` );`)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			stmt := prog.Statements[0].(*ast.CreateRegularExpressionStmt)
			if stmt.Expression != tt.want {
				t.Errorf("Expression = %q, want %q", stmt.Expression, tt.want)
			}
		})
	}
}

func TestCreateRegularExpression_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY REGULAR EXPRESSION Val.R ( Expression: '^a$' );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if !prog.Statements[0].(*ast.CreateRegularExpressionStmt).CreateOrModify {
		t.Error("CreateOrModify not set")
	}
}

func TestDropShowDescribeRegularExpression(t *testing.T) {
	prog, errs := Build(`DROP REGULAR EXPRESSION Val.R;
		SHOW REGULAR EXPRESSIONS;
		LIST REGULAR EXPRESSIONS IN Val;
		DESCRIBE REGULAR EXPRESSION Val.R;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(prog.Statements))
	}
	if _, ok := prog.Statements[0].(*ast.DropRegularExpressionStmt); !ok {
		t.Errorf("statement 0 = %T", prog.Statements[0])
	}
	if _, ok := prog.Statements[1].(*ast.ShowRegularExpressionsStmt); !ok {
		t.Errorf("statement 1 = %T", prog.Statements[1])
	}
	s2, ok := prog.Statements[2].(*ast.ShowRegularExpressionsStmt)
	if !ok {
		t.Fatalf("statement 2 = %T", prog.Statements[2])
	}
	if s2.Module != "Val" {
		t.Errorf("Module = %q", s2.Module)
	}
	if _, ok := prog.Statements[3].(*ast.DescribeRegularExpressionStmt); !ok {
		t.Errorf("statement 3 = %T", prog.Statements[3])
	}
}

// TestRegexKeywordsStillUsableAsIdentifiers guards the cost of adding REGULAR
// and EXPRESSIONS to the lexer: a new keyword stops being usable as an ordinary
// name unless it is also listed in the `keyword` rule. "expression" in
// particular is a plausible attribute name.
func TestRegexKeywordsStillUsableAsIdentifiers(t *testing.T) {
	prog, errs := Build(`CREATE ENTITY Val.Rule ( expression: String(200), regular: Boolean, expressions: Integer );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) != 3 {
		t.Fatalf("expected 3 attributes, got %d", len(stmt.Attributes))
	}
	for i, want := range []string{"expression", "regular", "expressions"} {
		if stmt.Attributes[i].Name != want {
			t.Errorf("attribute %d = %q, want %q", i, stmt.Attributes[i].Name, want)
		}
	}
}
