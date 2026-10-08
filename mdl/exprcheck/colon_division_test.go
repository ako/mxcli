// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"fmt"
	"strings"
	"testing"
)

// `:` is Mendix division — the same operator as `div`, always Decimal
// (docs.mendix.com/refguide/arithmetic-expressions: "You can use either the
// `div` or colon ( `:` ) syntax"). The checker's lexer did not know the
// character: it became a TokError, parseMul did not see an operator, parseCmp
// did not see a comparison, and Parse deliberately ignores a trailing TokError
// — so `hoursBetween(…) : 4 < $Max` was checked as just `hoursBetween(…)`, a
// Decimal, and a Boolean decision slot reported E009 on an expression Studio
// Pro and mxbuild accept (Evora, ConversationalUI.*_GetGranularity).

// shape renders a parse tree as an S-expression so precedence is asserted
// directly rather than inferred from a type.
func shape(e RobustExpr) string {
	switch n := e.(type) {
	case *BinExpr:
		return fmt.Sprintf("(%s %s %s)", n.Op, shape(n.L), shape(n.R))
	case *UnaryExpr:
		return fmt.Sprintf("(%s %s)", n.Op, shape(n.Operand))
	case *ParenExpr:
		return shape(n.Inner)
	case *VariableExpr:
		return n.Name
	case *NumberLit:
		return n.Value
	case *AttributePathExpr:
		return n.Variable + "/" + strings.Join(n.Path, "/")
	case *CallExpr:
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = shape(a)
		}
		return n.Name + "(" + strings.Join(args, ", ") + ")"
	case *RecoveredExpr:
		return "<recovered " + n.SourceFragment + ">"
	}
	return fmt.Sprintf("<%T>", e)
}

func numericScope(c *Context) {
	c.Scope = mapScope{
		"Max": KindInteger, "a": KindInteger, "b": KindInteger,
		"c": KindInteger, "d": KindInteger,
	}
}

func parseTree(src string) RobustExpr {
	ctx := Context{}
	numericScope(&ctx)
	e, _ := NewParser().Parse(src, ctx)
	return e
}

// The exact Evora expression, in the Boolean slot it sits in.
func TestColonDivision_EvoraGranularityConditionIsBoolean(t *testing.T) {
	const src = "hoursBetween($TokenMonitorSettings/DateTo, $TokenMonitorSettings/DateFrom) : 4 < $Max"
	hs := parseIn(t, src, "IfStmt.Condition", numericScope)
	if len(hs) != 0 {
		t.Errorf("valid Mendix condition reported hints %v: %+v", codes(hs), hs)
	}
	want := "(< (: hoursBetween(TokenMonitorSettings/DateTo, TokenMonitorSettings/DateFrom) 4) Max)"
	if got := shape(parseTree(src)); got != want {
		t.Errorf("parse tree\n got  %s\n want %s", got, want)
	}
}

func TestColonDivision_Precedence(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// `:` without a comparison.
		{"$a : $b", "(: a b)"},
		// Multiplicative binds tighter than additive…
		{"$a + $b : $c < $d", "(< (+ a (: b c)) d)"},
		{"$a - $b : $c", "(- a (: b c))"},
		{"$a : $b - $c", "(- (: a b) c)"},
		// …and all multiplicative operators share one left-associative level.
		{"$a : $b * $c", "(* (: a b) c)"},
		{"$a * $b : $c", "(: (* a b) c)"},
		{"$a : $b div $c", "(div (: a b) c)"},
		{"$a mod $b : $c", "(: (mod a b) c)"},
		// CONTROLS: the operators that already worked.
		{"$a div 4 < $Max", "(< (div a 4) Max)"},
		{"$a mod 4 < $Max", "(< (mod a 4) Max)"},
		{"$a + $b * $c < $d", "(< (+ a (* b c)) d)"},
	} {
		if got := shape(parseTree(tc.src)); got != tc.want {
			t.Errorf("%q\n got  %s\n want %s", tc.src, got, tc.want)
		}
	}
}

func TestColonDivision_TypesLikeDiv(t *testing.T) {
	ctx := Context{}
	numericScope(&ctx)
	for _, tc := range []struct {
		src  string
		want TypeKind
	}{
		{"$a : $b", KindDecimal}, // Integer : Integer is Decimal, as with div
		{"$a div $b", KindDecimal},
		{"$a mod $b", KindInteger},
		{"$a + $b : $c < $d", KindBoolean},
		{"$a : $b * $c", KindDecimal},
	} {
		if got := inferKind(parseTree(tc.src), ctx); got != tc.want {
			t.Errorf("%q: kind %s, want %s", tc.src, typeKindName(got), typeKindName(tc.want))
		}
	}
	// The Boolean slot accepts each comparison, and still rejects a bare
	// division (the rule is not simply switched off).
	for _, src := range []string{"$a : 4 < $Max", "$a div 4 < $Max", "$a mod 4 < $Max", "$a + $b : $c < $d"} {
		if hs := parseIn(t, src, "IfStmt.Condition", numericScope); hasCode(hs, "E009") {
			t.Errorf("%q: unexpected E009: %v", src, codes(hs))
		}
	}
	if hs := parseIn(t, "$a : 4", "IfStmt.Condition", numericScope); !hasCode(hs, "E009") {
		t.Errorf("a bare division in a Boolean slot must still be E009: %v", codes(hs))
	}
	if !SourceIsArithmeticDecimal("$a : $b", map[string]TypeKind{"a": KindInteger, "b": KindInteger}) {
		t.Error("SourceIsArithmeticDecimal does not treat `:` as division")
	}
}
