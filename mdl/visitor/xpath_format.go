// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// DefaultXPathWidth is the column a constraint is kept within when it can be.
// Studio Pro's constraint editor is a plain multi-line text box that does not
// wrap, so a 400-character filter is read by scrolling sideways — which is what
// upstream #979 is actually about.
const DefaultXPathWidth = 80

// FormatXPathConstraint renders a stored XPath constraint so it can be read in
// Studio Pro's constraint editor, breaking it across lines when it does not fit
// in DefaultXPathWidth columns.
//
// The form is CANONICAL, not preserved. #979 was filed as "keep the whitespace I
// typed", but the formatting that matters to the reporter is the formatting
// Studio Pro's editor holds, and a constraint is rebuilt from its parse tree on
// every write (see buildXPathString) — so by the time anything is stored there is
// no original whitespace left to keep. Deriving the layout from the expression
// instead means a constraint comes out readable however it went in, including one
// a person formatted by hand in Studio Pro.
//
// Two properties make it safe to apply on every write:
//
//   - A constraint that already fits comes back BYTE-IDENTICAL, so the common
//     case is untouched and ADR-0008's write elision still skips the unit.
//   - It is idempotent, so re-running it over its own output moves nothing.
//
// Anything the grammar cannot read is handed back unchanged. A formatter that
// rewrites what it cannot parse is a data-loss bug wearing a tidy name.
func FormatXPathConstraint(constraint string) string {
	return FormatXPathConstraintWidth(constraint, DefaultXPathWidth)
}

// FormatXPathConstraintWidth is FormatXPathConstraint with an explicit column
// budget. A width of 0 or less means DefaultXPathWidth.
func FormatXPathConstraintWidth(constraint string, width int) string {
	if width <= 0 {
		width = DefaultXPathWidth
	}
	if strings.TrimSpace(constraint) == "" {
		return constraint
	}
	// Already short enough to read at a glance: leave it exactly as it is. This
	// is the case for the overwhelming majority of constraints, and returning the
	// caller's own bytes is what keeps this change from touching them.
	if !strings.ContainsAny(constraint, "\r\n") && len(constraint) <= width {
		return constraint
	}
	// Sibling predicate groups (`[a][b]`) are the author's own top-level
	// conjunction, so each gets its own line and is laid out in its own right.
	// SplitXPathPredicateGroups returns nil for anything that is not a bracketed
	// constraint at all.
	groups := SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		return constraint
	}
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, formatXPathGroup(g, width))
	}
	return strings.Join(out, "\n")
}

// formatXPathGroup lays out one bracketed group.
func formatXPathGroup(group string, width int) string {
	expr, ok := ParseXPathConstraint(group)
	if !ok {
		return strings.TrimSpace(group)
	}
	one := "[" + xpathExprToString(expr) + "]"
	if len(one) <= width {
		return one
	}

	var b strings.Builder
	if !writeXPathBody(&b, expr, width, "  ") {
		// No boolean joint anywhere near the top — a single long comparison, or a
		// long path. Opening brackets around it would add lines without adding
		// information, and cutting it anywhere else produces a constraint Mendix
		// rejects. It goes out whole and over width.
		return one
	}
	return "[\n" + b.String() + "]"
}

// writeXPathBody writes the inside of a group, reporting whether the expression
// had any structure worth opening up.
func writeXPathBody(b *strings.Builder, expr ast.Expression, width int, indent string) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if isXPathBoolChain(e) {
			writeXPathChain(b, e, width, indent)
			return true
		}
	case *ast.ParenExpr:
		// `[(a and b and …)]` — the outer parentheses are the bracket's own
		// grouping restated. Opening the chain directly says the same thing with
		// one level less indentation.
		if isXPathBoolChain(e.Inner) {
			writeXPathChain(b, e.Inner, width, indent)
			return true
		}
	case *ast.UnaryExpr:
		if inner, ok := xpathNotChain(e); ok {
			b.WriteString(indent + "not(\n")
			writeXPathChain(b, inner, width, indent+"  ")
			b.WriteString(indent + ")\n")
			return true
		}
	}
	return false
}

// writeXPathChain writes the operands of an and/or chain, one per line, with the
// operator leading each continuation line.
func writeXPathChain(b *strings.Builder, expr ast.Expression, width int, indent string) {
	op, operands := flattenXPathBoolChain(expr)
	if op == "" {
		b.WriteString(indent + xpathExprToString(expr) + "\n")
		return
	}
	for i, operand := range operands {
		prefix := ""
		if i > 0 {
			prefix = op + " "
		}
		writeXPathOperand(b, operand, width, indent, prefix, op)
	}
}

// writeXPathOperand writes one operand of a chain.
func writeXPathOperand(b *strings.Builder, operand ast.Expression, width int, indent, prefix, chainOp string) {
	one := xpathExprToString(operand)

	// A sub-chain joined by the OTHER operator gets explicit parentheses. Mendix
	// binds `and` tighter than `or`, so `a or b and c` is not what most readers
	// see at a glance — and a filter is being broken across lines precisely
	// because it had stopped being obvious. The parentheses are the grouping the
	// parse tree already has, written down.
	if subOp, _ := flattenXPathBoolChain(operand); subOp != "" && subOp != chainOp {
		if len(indent)+len(prefix)+len(one)+2 <= width {
			b.WriteString(indent + prefix + "(" + one + ")\n")
			return
		}
		writeXPathBlock(b, operand, width, indent, prefix+"(")
		return
	}

	if len(indent)+len(prefix)+len(one) <= width {
		b.WriteString(indent + prefix + one + "\n")
		return
	}

	// Too wide: look for a joint one level down.
	switch e := operand.(type) {
	case *ast.ParenExpr:
		if isXPathBoolChain(e.Inner) {
			writeXPathBlock(b, e.Inner, width, indent, prefix+"(")
			return
		}
	case *ast.UnaryExpr:
		if inner, ok := xpathNotChain(e); ok {
			writeXPathBlock(b, inner, width, indent, prefix+"not(")
			return
		}
	}

	// No joint: a long path or a long comparison, written whole.
	b.WriteString(indent + prefix + one + "\n")
}

// writeXPathBlock writes an opener, an indented chain, and its closing paren.
func writeXPathBlock(b *strings.Builder, inner ast.Expression, width int, indent, opener string) {
	b.WriteString(indent + opener + "\n")
	writeXPathChain(b, inner, width, indent+"  ")
	b.WriteString(indent + ")\n")
}

// xpathNotChain reports whether a unary expression is `not(...)` wrapped around a
// boolean chain, and returns that chain.
func xpathNotChain(e *ast.UnaryExpr) (ast.Expression, bool) {
	if !strings.EqualFold(e.Operator, "not") {
		return nil, false
	}
	inner := e.Operand
	if p, ok := inner.(*ast.ParenExpr); ok {
		inner = p.Inner
	}
	if !isXPathBoolChain(inner) {
		return nil, false
	}
	return inner, true
}

// flattenXPathBoolChain returns the top-level boolean operator and every operand
// joined by it. `a and b and c` is three operands, not a nest of two.
//
// Only the top operator is flattened: in `a and b or c` the top node is the `or`,
// so the operands are `a and b` and `c`, and the conjunction stays one operand to
// be parenthesised or opened up on its own. Mendix's precedence (and binds
// tighter than or) is the parser's, and this walk inherits it rather than
// restating it.
func flattenXPathBoolChain(expr ast.Expression) (string, []ast.Expression) {
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return "", nil
	}
	op := strings.ToLower(be.Operator)
	if op != "and" && op != "or" {
		return "", nil
	}
	var operands []ast.Expression
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		if b, ok := e.(*ast.BinaryExpr); ok && strings.EqualFold(b.Operator, op) {
			walk(b.Left)
			walk(b.Right)
			return
		}
		operands = append(operands, e)
	}
	walk(be)
	return op, operands
}

// isXPathBoolChain reports whether an expression is joined by and/or at its top.
func isXPathBoolChain(expr ast.Expression) bool {
	op, _ := flattenXPathBoolChain(expr)
	return op != ""
}
