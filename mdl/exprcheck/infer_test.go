// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

func TestInferSourceKind_Division(t *testing.T) {
	vars := map[string]TypeKind{"a": KindInteger, "b": KindInteger, "d": KindDecimal}
	cases := []struct {
		src  string
		want TypeKind
	}{
		{"$a div $b", KindDecimal},       // integer div integer = Decimal
		{"$a * 100 div $b", KindDecimal}, // outer div dominates
		{"$a + $b", KindInteger},         // integer + integer = Integer
		{"$a * $b", KindInteger},         // integer * integer = Integer
		{"$d * $a", KindDecimal},         // Decimal is contagious
		{"$a mod $b", KindInteger},       // integer mod integer = Integer
	}
	for _, tc := range cases {
		if got := InferSourceKind(tc.src, vars); got != tc.want {
			t.Errorf("InferSourceKind(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSourceIsArithmeticDecimal(t *testing.T) {
	vars := map[string]TypeKind{"a": KindInteger, "b": KindInteger, "n": KindDecimal}
	yes := []string{
		"$a div $b",
		"$a * 100 div $b",
		"($a div $b)",
		"$a div $b * 100",
		"$n + $a", // Decimal operand → Decimal arithmetic
	}
	for _, src := range yes {
		if !SourceIsArithmeticDecimal(src, vars) {
			t.Errorf("SourceIsArithmeticDecimal(%q) = false, want true", src)
		}
	}
	no := []string{
		"round(sqrt($a))",  // function root — Mendix accepts into Integer
		"round($a div $b)", // rounding wrapper is accepted
		"floor($a div $b)",
		"$a + $b", // integer arithmetic → Integer
		"$a * $b", // integer arithmetic → Integer
		"$a",      // bare variable, not arithmetic
	}
	for _, src := range no {
		if SourceIsArithmeticDecimal(src, vars) {
			t.Errorf("SourceIsArithmeticDecimal(%q) = true, want false", src)
		}
	}
}
