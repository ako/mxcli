// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// TestIntegerTargetAgreesWithMxbuild pins MDL041's Decimal inference to what
// Mendix itself accepts in an Integer target. Every row was measured against
// mxbuild 11.13.0 by writing the expression into a microflow with `mxcli exec
// --no-check` and running `mx check` (sudoku finding #49).
//
// The first row is the control, and it is what makes the rest of the table
// meaningful: `mx check` really does report CE0117 for this class, so the rows
// that build at 0 errors are a result rather than a validator that never fires.
//
// The defect this guards: round/floor/ceil were declared Decimal-returning and
// exempted only when they were the whole right-hand side. So `round($a div $b)`
// passed and `round($a div $b) * 3` — which mxbuild accepts — was refused, and
// since exec refuses to write when any rule errors, a microflow that builds
// clean could be read out of the model with `describe` and not written back.
func TestIntegerTargetAgreesWithMxbuild(t *testing.T) {
	vars := map[string]TypeKind{"a": KindInteger, "b": KindInteger, "d": KindDecimal}
	cases := []struct {
		src      string
		rejected bool // what mxbuild 11.13.0 says: true = CE0117
	}{
		// Control: the class MDL041 exists for.
		{"$a div $b", true},

		// Rounding functions yield whole numbers wherever they appear, not only
		// as the whole right-hand side.
		{"round($a div $b)", false},
		{"round(floor($a div $b))", false},
		{"round(floor($a div $b)) * 3", false},
		{"3 * round(floor($a div $b))", false},
		{"round($a div $b) + 3", false},
		{"round($a div $b) mod 3", false},
		{"floor($a div $b)", false},
		{"floor($a div $b) * 3", false},
		{"ceil($a div $b) * 3", false},

		// Two-argument round() keeps its decimals, so it is genuinely Decimal.
		{"round($a div $b, 2)", true},

		// abs/max/min preserve their operands' kind rather than always widening.
		{"abs($a)", false},
		{"max($a, 3)", false},
		{"min($a, 3)", false},
		{"abs($d)", true},
		{"max($a, 3.5)", true},

		// Genuinely Decimal-returning built-ins stay flagged.
		{"pow($a, 2)", true},
		{"sqrt($a)", true},
		{"random()", true},

		// Unrelated Integer-returning built-in, to show the fix is not blanket
		// permissiveness in arithmetic.
		{"length('abc') * 3", false},
	}
	for _, tc := range cases {
		if got := SourceRejectedForIntegerTarget(tc.src, vars); got != tc.rejected {
			t.Errorf("SourceRejectedForIntegerTarget(%q) = %v, mxbuild 11.13.0 says rejected=%v",
				tc.src, got, tc.rejected)
		}
	}
}

// TestRoundReturnKindFollowsArity guards the mechanism behind the table above:
// the kind is resolved per call site, so it is right for a nested call too.
func TestRoundReturnKindFollowsArity(t *testing.T) {
	sig := funcTable["round"]
	if got := sig.retFor(1); got != KindInteger {
		t.Errorf("round/1 returns %v, want Integer", got)
	}
	if got := sig.retFor(2); got != KindDecimal {
		t.Errorf("round/2 returns %v, want Decimal", got)
	}
	for _, name := range []string{"floor", "ceil"} {
		if got := funcTable[name].retFor(1); got != KindInteger {
			t.Errorf("%s returns %v, want Integer", name, got)
		}
	}
	for _, name := range []string{"abs", "max", "min"} {
		if !funcTable[name].retFromArgs {
			t.Errorf("%s must take its kind from its operands", name)
		}
	}
}
