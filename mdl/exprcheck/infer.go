// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "strings"

// InferSourceKind parses a Mendix expression source string and returns the
// inferred result TypeKind, or KindUnknown when it cannot be determined.
//
// vars maps variable names (without the leading '$') to their kinds so operands
// such as $count resolve during inference; pass nil when no scope is available.
// This is a lightweight entry point for assignment-type checks (e.g. detecting
// that an Integer variable is being assigned a Decimal `div` result) without
// wiring the full slot/catalog machinery.
func InferSourceKind(src string, vars map[string]TypeKind) TypeKind {
	if strings.TrimSpace(src) == "" {
		return KindUnknown
	}
	ctx := Context{}
	if vars != nil {
		ctx.Scope = mapScope(vars)
	}
	root, _ := (&parserImpl{}).Parse(src, ctx)
	if root == nil {
		return KindUnknown
	}
	return inferKind(root, ctx)
}

// SourceIsArithmeticDecimal reports whether src is a bare arithmetic expression
// whose result kind is Decimal — the case that fails mx check when assigned to
// an Integer/Long variable (CE0117), most commonly `$a * 100 div $b`.
//
// It deliberately fires only when the outermost operation (after unwrapping
// parentheses) is arithmetic (+, -, *, div, mod). An expression whose root is a
// function call — round(), floor(), ceil(), trunc(), sqrt() … — is NOT flagged,
// because Mendix accepts those whole-number results in an Integer target even
// though the functions are typed Decimal. This keeps the check aimed at genuine
// fractional-division mistakes without false positives on rounding.
func SourceIsArithmeticDecimal(src string, vars map[string]TypeKind) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	ctx := Context{}
	if vars != nil {
		ctx.Scope = mapScope(vars)
	}
	root, _ := (&parserImpl{}).Parse(src, ctx)
	root = unwrapParens(root)
	be, ok := root.(*BinExpr)
	if !ok {
		return false
	}
	switch be.Op {
	case "+", "-", "*", "div", "mod":
		return inferKind(be, ctx) == KindDecimal
	}
	return false
}

// unwrapParens strips redundant parentheses from the outside of an expression.
func unwrapParens(e RobustExpr) RobustExpr {
	for {
		p, ok := e.(*ParenExpr)
		if !ok {
			return e
		}
		e = p.Inner
	}
}

// mapScope is a trivial Scope backed by a name→kind map. Keys are variable
// names without the leading '$'.
type mapScope map[string]TypeKind

func (m mapScope) Lookup(name string) (TypeKind, bool) {
	k, ok := m[strings.TrimPrefix(name, "$")]
	return k, ok
}
