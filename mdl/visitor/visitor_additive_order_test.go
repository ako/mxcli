// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ledger #105 — an additive chain must keep the operators in the order they were
// written. buildAdditiveExpression used to read AllPLUS() and AllMINUS() as two
// separate token lists and emit every `+` before every `-`, so `$A - $B + 1` was
// REBUILT as `$A + $B - 1` and stored that way.
//
// This is the worst shape a defect can take here: the rewritten expression is
// perfectly valid, so `mxcli check`, `mx check` and the build are all green, and
// the only symptom is that the running app computes a different number. The
// ledger caught it because a month span rendered as 48620.
//
// The cases are the ledger's own probe. Note which ones survived the bug —
// `$A - $B - 1` (no `+` to float ahead) and `$A + $B - 1` (already in that
// order) — because a test built only from failing cases would have passed
// against an implementation that emits all minuses first instead.
func TestAdditiveChainKeepsWrittenOperatorOrder(t *testing.T) {
	cases := []struct {
		expr string
		want []string // operators, in source order
	}{
		{"$A - $B", []string{"-"}},
		{"$A + $B", []string{"+"}},
		{"$A - $B + 1", []string{"-", "+"}},           // swapped before the fix
		{"$A - $B - 1", []string{"-", "-"}},           // survived the bug
		{"$A + $B - 1", []string{"+", "-"}},           // survived the bug
		{"$A - $B + $C", []string{"-", "+"}},          // swapped before the fix
		{"$A - $B + $C - 2", []string{"-", "+", "-"}}, // swapped before the fix
		{"1 - $A + $B", []string{"-", "+"}},           // swapped before the fix
		{"$A + $B + $C", []string{"+", "+"}},
		{"$A - $B - $C + 1 + 2", []string{"-", "-", "+", "+"}},
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			src := "create microflow M.Mf ($A: integer, $B: integer, $C: integer) returns integer as $R begin declare $R integer = 0; $R = " +
				tc.expr + "; return $R; end"
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors for %q: %v", tc.expr, errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

			var assigned ast.Expression
			for _, s := range mf.Body {
				if set, ok := s.(*ast.MfSetStmt); ok {
					assigned = set.Value
				}
			}
			if assigned == nil {
				t.Fatalf("no assignment found in %q", src)
			}

			got := additiveOperators(assigned)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("%q built operators %v, want %v (source order)", tc.expr, got, tc.want)
			}
		})
	}
}

// TestAdditiveOperatorsIgnoresTighterBinding pins the reason `$A - $B * 2` was
// never affected: multiplication does not join the additive chain, so the
// helper below must not walk into it. Without this, a regression in the
// multiplicative builder would show up as a confusing failure in the additive
// test rather than its own.
func TestAdditiveOperatorsIgnoresTighterBinding(t *testing.T) {
	prog, errs := Build("create microflow M.Mf ($A: integer, $B: integer) returns integer as $R begin declare $R integer = 0; $R = $A - $B * 2; return $R; end")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	for _, s := range mf.Body {
		if set, ok := s.(*ast.MfSetStmt); ok {
			if got := additiveOperators(set.Value); strings.Join(got, ",") != "-" {
				t.Errorf("`$A - $B * 2` additive operators = %v, want [-]", got)
			}
		}
	}
}

// additiveOperators flattens a left-nested BinaryExpr chain of + and - into the
// operators in source order. It stops at any other operator, so a tighter-binding
// sub-expression is one opaque operand rather than part of the chain.
func additiveOperators(e ast.Expression) []string {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok || (bin.Operator != "+" && bin.Operator != "-") {
		return nil
	}
	return append(additiveOperators(bin.Left), bin.Operator)
}
