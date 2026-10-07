// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// parseXPathConstraint parses an XPath constraint string like "[expr]" and returns
// the AST Expression. Uses the ANTLR parser directly.
func parseXPathConstraint(input string) ast.Expression {
	is := antlr.NewInputStream(input)
	lexer := parser.NewMDLLexer(is)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()

	ctx := p.XpathConstraint()
	xcCtx := ctx.(*parser.XpathConstraintContext)
	if xpathExpr := xcCtx.XpathExpr(); xpathExpr != nil {
		return buildXPathExpr(xpathExpr)
	}
	return nil
}

// roundTripXPath parses an XPath constraint and serializes it back using xpathExprToString.
func roundTripXPath(input string) string {
	expr := parseXPathConstraint(input)
	if expr == nil {
		return ""
	}
	return "[" + xpathExprToString(expr) + "]"
}

func TestXPath_SimpleAttributeComparison(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"string equality", "[Name = 'John']", "[Name = 'John']"},
		{"not equal", "[State != 'Aborted']", "[State != 'Aborted']"},
		{"less than", "[Age < 30]", "[Age < 30]"},
		{"greater equal", "[Price >= 10]", "[Price >= 10]"},
		{"variable comparison", "[Name = $Username]", "[Name = $Username]"},
		{"empty comparison", "[DueDate != empty]", "[DueDate != empty]"},
		{"boolean true", "[Active = true]", "[Active = true]"},
		{"boolean false", "[Deleted = false]", "[Deleted = false]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_BooleanOperators(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"and", "[A = 1 and B = 2]", "[A = 1 and B = 2]"},
		{"or", "[A = 1 or B = 2]", "[A = 1 or B = 2]"},
		{"grouped or", "[(A = 1 or B = 2)]", "[(A = 1 or B = 2)]"},
		{"and with or", "[A = 1 and (B = 2 or C = 3)]", "[A = 1 and (B = 2 or C = 3)]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_BareAssociationPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"qualified name comparison",
			"[Module.Association = $object]",
			"[Module.Association = $object]",
		},
		{
			"two-hop path",
			"[Module.Assoc/Module.Entity/Attr = $val]",
			"[Module.Assoc/Module.Entity/Attr = $val]",
		},
		{
			"three-hop path",
			"[A.B/C.D/E.F/Name = 'test']",
			"[A.B/C.D/E.F/Name = 'test']",
		},
		{
			"path existence check",
			"[Module.Assoc/Module.Entity]",
			"[Module.Assoc/Module.Entity]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_VariablePaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"variable with attribute",
			"[$var/Name = 'test']",
			"[$var/Name = 'test']",
		},
		{
			"variable with association path",
			"[$var/Module.Assoc/Module.Entity/Attr = $other]",
			"[$var/Module.Assoc/Module.Entity/Attr = $other]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_NestedPredicates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"simple nested predicate",
			"[Module.Assoc/Module.Entity[State = 'Active']]",
			"[Module.Assoc/Module.Entity[State = 'Active']]",
		},
		{
			"nested predicate with further path",
			"[Module.Assoc/Module.Entity[State = 'Active']/SubAssoc = $val]",
			"[Module.Assoc/Module.Entity[State = 'Active']/SubAssoc = $val]",
		},
		{
			"reversed modifier",
			"[System.roles[reversed()]/System.UserRole = $role]",
			"[System.roles[reversed()]/System.UserRole = $role]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_NotExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"not existence",
			"[not(Module.Assoc/Module.Entity)]",
			"[not(Module.Assoc/Module.Entity)]",
		},
		{
			"not boolean",
			"[not(IsDraft)]",
			"[not(IsDraft)]",
		},
		{
			"not with contains",
			"[not(contains(Name, 'demo'))]",
			"[not(contains(Name, 'demo'))]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_Functions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"contains",
			"[contains(Name, $SearchStr)]",
			"[contains(Name, $SearchStr)]",
		},
		{
			"starts-with",
			"[starts-with(Name, $prefix)]",
			"[starts-with(Name, $prefix)]",
		},
		{
			"true function",
			"[Active = true()]",
			"[Active = true()]",
		},
		{
			"false function",
			"[Displayed = false()]",
			"[Displayed = false()]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_Tokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"current user token",
			"[System.owner = [%CurrentUser%]]",
			"[System.owner = [%CurrentUser%]]",
		},
		{
			"current datetime token",
			"[DueDate < [%CurrentDateTime%]]",
			"[DueDate < [%CurrentDateTime%]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_IdPseudoAttribute(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"id equals variable",
			"[id = $currentUser]",
			"[id = $currentUser]",
		},
		{
			"id not equals",
			"[id != $existingObject]",
			"[id != $existingObject]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"multiple conditions with empty check",
			"[State = 'Completed' and ($IgnoreAfter or EndTime >= $After)]",
			"[State = 'Completed' and ($IgnoreAfter or EndTime >= $After)]",
		},
		{
			"bare boolean attribute",
			"[Active]",
			"[Active]",
		},
		{
			"system owner with token string",
			"[System.owner = '[%CurrentUser%]']",
			"[System.owner = '[%CurrentUser%]']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_EnumValueReference(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			// The serializer does not know the constrained entity, so it cannot
			// tell an enumeration value from a qualified attribute; the writer
			// makes the value 'Rectified' (storedXPathConstraint, #874).
			"3-part name kept for the writer to resolve",
			"[Status = BST.ComplianceStatus.Rectified]",
			"[Status = BST.ComplianceStatus.Rectified]",
		},
		{
			"2-part qualified name preserved",
			"[Module.Association = $object]",
			"[Module.Association = $object]",
		},
		{
			"string literal enum preserved",
			"[Status = 'Active']",
			"[Status = 'Active']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripXPath(tt.input)
			if got != tt.want {
				t.Errorf("roundTripXPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestXPath_ASTTypes(t *testing.T) {
	t.Run("bare path creates XPathPathExpr", func(t *testing.T) {
		expr := parseXPathConstraint("[Module.Assoc/Module.Entity/Attr = $val]")
		binExpr, ok := expr.(*ast.BinaryExpr)
		if !ok {
			t.Fatalf("expected BinaryExpr, got %T", expr)
		}
		pathExpr, ok := binExpr.Left.(*ast.XPathPathExpr)
		if !ok {
			t.Fatalf("expected XPathPathExpr for left side, got %T", binExpr.Left)
		}
		if len(pathExpr.Steps) != 3 {
			t.Errorf("expected 3 steps, got %d", len(pathExpr.Steps))
		}
	})

	t.Run("single identifier stays as IdentifierExpr", func(t *testing.T) {
		expr := parseXPathConstraint("[Active = true]")
		binExpr, ok := expr.(*ast.BinaryExpr)
		if !ok {
			t.Fatalf("expected BinaryExpr, got %T", expr)
		}
		_, ok = binExpr.Left.(*ast.IdentifierExpr)
		if !ok {
			t.Fatalf("expected IdentifierExpr for 'Active', got %T", binExpr.Left)
		}
	})

	t.Run("nested predicate creates XPathPathExpr with predicate", func(t *testing.T) {
		expr := parseXPathConstraint("[Module.Assoc/Module.Entity[Active]]")
		pathExpr, ok := expr.(*ast.XPathPathExpr)
		if !ok {
			t.Fatalf("expected XPathPathExpr, got %T", expr)
		}
		if len(pathExpr.Steps) != 2 {
			t.Errorf("expected 2 steps, got %d", len(pathExpr.Steps))
		}
		if len(pathExpr.Steps[1].Predicates) == 0 {
			t.Error("expected predicate on second step")
		}
	})

	t.Run("not creates UnaryExpr", func(t *testing.T) {
		expr := parseXPathConstraint("[not(Active)]")
		unaryExpr, ok := expr.(*ast.UnaryExpr)
		if !ok {
			t.Fatalf("expected UnaryExpr, got %T", expr)
		}
		if unaryExpr.Operator != "not" {
			t.Errorf("expected operator 'not', got %q", unaryExpr.Operator)
		}
	})
}

// TestXPath_NegativeNumericLiteral pins issuetracker finding #18.
//
// `-` was admissible as an xpathWord, so `[Amount > -7]` parsed the sign as a
// name and left the digits stranded — `extraneous input '7'`. The report called
// this "negative literals truncate to -", which is what it looks like from the
// outside. Adding unary minus to the grammar without a matching visitor case
// would have been worse than the parse error: the constraint would parse and
// silently serialize to `[Amount > ]`.
func TestXPath_NegativeNumericLiteral(t *testing.T) {
	t.Run("round-trips with the sign intact", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{"negative integer", "[Amount > -7]", "[Amount > -7]"},
			{"negative decimal", "[Amount <= -12.5]", "[Amount <= -12.5]"},
			{"negative on the left", "[-7 < Amount]", "[-7 < Amount]"},
			{"compound with and", "[Amount > -7 and Code = 'A']", "[Amount > -7 and Code = 'A']"},
			{"function argument", "[contains(Code, 'A') and Amount > -1]", "[contains(Code, 'A') and Amount > -1]"},
			// Regression: MINUS was removed from xpathWord, so hyphenated XPath
			// function names must still resolve (they lex as one HYPHENATED_ID).
			{"hyphenated function still parses", "[starts-with(Code, 'AB')]", "[starts-with(Code, 'AB')]"},
			// Regression: positive literals unaffected.
			{"positive integer", "[Amount > 7]", "[Amount > 7]"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := roundTripXPath(tc.input); got != tc.want {
					t.Errorf("roundTripXPath(%q) = %q, want %q", tc.input, got, tc.want)
				}
			})
		}
	})

	t.Run("builds a UnaryExpr, not a dropped operand", func(t *testing.T) {
		expr := parseXPathConstraint("[Amount > -7]")
		bin, ok := expr.(*ast.BinaryExpr)
		if !ok {
			t.Fatalf("expected BinaryExpr, got %T", expr)
		}
		unary, ok := bin.Right.(*ast.UnaryExpr)
		if !ok {
			t.Fatalf("right operand = %T, want *ast.UnaryExpr — a nil operand serializes to an empty string", bin.Right)
		}
		if unary.Operator != "-" {
			t.Errorf("operator = %q, want -", unary.Operator)
		}
		if unary.Operand == nil {
			t.Error("operand is nil — the literal was dropped")
		}
	})
}

// Two predicates on one path step — `Entity[a][b]` — are XPath, and Mendix
// accepts them in a retrieve and an access rule alike. The step rule admitted
// only one, so the second `[` was a parse error: "missing ']' at '['" when the
// path ended on the step, "mismatched input '/' expecting ';'" when it went on
// (mendixlabs/mxcli#1281). Each predicate is kept as written, never folded into
// one `and`: `[reversed()]` is a step predicate too, and is not a condition.
func TestXPath_ConsecutiveStepPredicates(t *testing.T) {
	t.Run("parses in a retrieve", func(t *testing.T) {
		for _, where := range []string{
			// the issue's two shapes: the path continues after the step, and ends on it
			"[M.UserAssignment_TenantUser/M.TenantUser[Status = 'Active'][AnonymizedAt = empty]/M.TenantUser_Account = '[%CurrentUser%]']",
			"[M.UserAssignment_TenantUser/M.TenantUser[Status = 'Active'][AnonymizedAt = empty]]",
		} {
			src := "create microflow M.ZZ_Two ()\nreturns Nothing\nbegin\n  retrieve $x from M.Location where " + where + ";\nend;\n/\n"
			if _, errs := Build(src); len(errs) > 0 {
				t.Errorf("where %s: %v", where, errs)
			}
		}
	})

	t.Run("round-trips every predicate", func(t *testing.T) {
		for _, in := range []string{
			"[M.Assoc/M.Entity[Status = 'Active'][AnonymizedAt = empty]/M.Other = $v]",
			"[M.Assoc/M.Entity[Status = 'Active'][AnonymizedAt = empty]]",
			"[M.Assoc/M.Entity[A = 1 or B = 2][C = 3][D = 4]]",
			"[System.roles[reversed()][Name = 'x']/System.UserRole = $role]",
		} {
			if got, ok := ParseXPathConstraint(in); !ok {
				t.Errorf("ParseXPathConstraint(%q) did not parse", in)
			} else if s := "[" + xpathExprToString(got) + "]"; s != in {
				t.Errorf("round trip of %q = %q", in, s)
			}
		}
	})
}
