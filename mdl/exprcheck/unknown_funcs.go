// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "strings"

// FuncRef identifies a function call found in an expression whose name is not a
// known Mendix built-in — a hallucinated or misspelled function (e.g. randomInt).
type FuncRef struct {
	Name       string
	Line       int
	Column     int
	Suggestion string // nearest known built-in, or "" if none is close
}

// UnknownFunctionCalls parses a Mendix expression and returns every call to a
// name that is not in the built-in function table (funcTable, which lists every
// Mendix expression function). A bare `name(...)` in a Mendix expression is
// always a built-in call — enumeration/entity references are qualified names, not
// calls — so an unknown name is a real error: it parses and passes a naive check
// but fails the build with CE0117 "Error(s) in expression". Findings #1.
func UnknownFunctionCalls(src string) []FuncRef {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	root, _ := (&parserImpl{}).Parse(src, Context{})
	if root == nil {
		return nil
	}
	var out []FuncRef
	walkCalls(root, func(c *CallExpr) {
		if c.Name == "" {
			return
		}
		if _, ok := funcTable[c.Name]; ok {
			return
		}
		out = append(out, FuncRef{
			Name:       c.Name,
			Line:       c.Pos().Line,
			Column:     c.Pos().Column,
			Suggestion: nearestFunc(c.Name),
		})
	})
	return out
}

// walkCalls invokes fn for every CallExpr in the tree (depth-first).
func walkCalls(e RobustExpr, fn func(*CallExpr)) {
	switch n := e.(type) {
	case *CallExpr:
		fn(n)
		for _, a := range n.Args {
			walkCalls(a, fn)
		}
	case *BinExpr:
		walkCalls(n.L, fn)
		walkCalls(n.R, fn)
	case *UnaryExpr:
		walkCalls(n.Operand, fn)
	case *ParenExpr:
		walkCalls(n.Inner, fn)
	case *IfThenElseExpr:
		walkCalls(n.Cond, fn)
		walkCalls(n.Then, fn)
		walkCalls(n.Else, fn)
	}
}

// nearestFunc returns the built-in whose name is closest to name — a prefix
// relationship (randomInt→random) or a small edit distance — for a "did you
// mean" hint. Returns "" when nothing is close enough to be worth suggesting.
func nearestFunc(name string) string {
	lower := strings.ToLower(name)
	best, bestDist := "", 1<<30
	for cand := range funcTable {
		cl := strings.ToLower(cand)
		// A prefix relationship is a strong signal (randomInt / random).
		if strings.HasPrefix(lower, cl) || strings.HasPrefix(cl, lower) {
			return cand
		}
		d := levenshtein(lower, cl)
		if d < bestDist {
			best, bestDist = cand, d
		}
	}
	// Suggest only a genuinely close match: within a third of the name length
	// (min 2), so unrelated names get no misleading suggestion.
	threshold := len(name) / 3
	if threshold < 2 {
		threshold = 2
	}
	if bestDist <= threshold {
		return best
	}
	return ""
}

// levenshtein is the standard edit distance between two strings.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// roundingFuncs are the Decimal-returning built-ins whose result Mendix DOES
// accept in an Integer/Long target (they yield a whole number). They must not be
// flagged by SourceRejectedForIntegerTarget.
//
// `trunc` used to be listed here. Mendix has no such built-in — `trunc($D)`
// fails the build with CE0117 on 11.13.0 — and listing it invited "fix" the
// MDL044 report by adding it to funcTable, which would let the bad expression
// through. MDL044 flags it, correctly.
var roundingFuncs = map[string]bool{
	"round": true, "floor": true, "ceil": true,
}

// SourceRejectedForIntegerTarget reports whether assigning src to an Integer/Long
// variable is rejected by Mendix (CE0117) because src is Decimal-typed. It covers
// two cases the build rejects but a naive check misses (findings #2):
//   - a bare arithmetic Decimal result (e.g. `$a * 100 div $b`); and
//   - a call to a Decimal-returning built-in that is NOT a rounding function
//     (e.g. random(), secondsBetween(...), div-family) — round/floor/ceil/trunc
//     are excluded because Mendix accepts their whole-number result.
func SourceRejectedForIntegerTarget(src string, vars map[string]TypeKind) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	ctx := Context{}
	if vars != nil {
		ctx.Scope = mapScope(vars)
	}
	root, _ := (&parserImpl{}).Parse(src, ctx)
	root = unwrapParens(root)
	switch n := root.(type) {
	case *BinExpr:
		switch n.Op {
		case "+", "-", "*", "div", "mod":
			return inferKind(n, ctx) == KindDecimal
		}
	case *CallExpr:
		if roundingFuncs[n.Name] {
			return false
		}
		if ret, ok := FuncReturnKind(n.Name); ok {
			return ret == KindDecimal
		}
	}
	return false
}
