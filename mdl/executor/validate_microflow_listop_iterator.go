// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkListOperationIterator flags a FILTER/FIND predicate that navigates from a
// variable which is not in scope — MDL-LISTOP01.
//
// Mendix binds the current item to `$currentObject` in a filter/find expression
// and defines no other iterator name. `filter($L, $item/Amount > 0)` therefore
// fails the build with CE0109 "Undefined variable 'item'", which mxcli used to
// pass through silently (issue #1002).
//
// The rule keys on scope, not on the name: `$item` is perfectly valid in a
// predicate when it is the enclosing loop's iterator, which is how the idiom in
// CLAUDE.md ("use FIND for an O(N) lookup") is written —
// `find($TargetList, key = $item/key)` navigates the OUTER loop's variable on the
// right-hand side. Flagging the name would break that; flagging an unbound
// variable does not. The variable set is collected flow-wide (branches and loop
// bodies do not open a scope in Mendix — see MDL063), so a predicate may
// reference anything the microflow binds anywhere.
//
// Unlike the executor's qualification of bare attribute names, this needs no
// project: it is answered entirely from the statement's own body, so plain
// `mxcli check` reports it.
func (v *microflowValidator) checkListOperationIterator(body []ast.MicroflowStatement) {
	inScope := map[string]bool{"currentObject": true}
	for _, p := range v.params {
		if p.Name != "" {
			inScope[p.Name] = true
		}
	}
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		for _, p := range statementProducedVars(s) {
			inScope[p.name] = true
		}
	})

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		lo, ok := s.(*ast.ListOperationStmt)
		if !ok || lo.Condition == nil {
			return
		}
		if lo.Operation != ast.ListOpFilter && lo.Operation != ast.ListOpFind {
			return
		}
		op := strings.ToLower(lo.Operation.String())
		unbound := map[string]bool{}
		for _, name := range predicatePathVariables(lo.Condition) {
			if !inScope[name] {
				unbound[name] = true
			}
		}
		names := make([]string, 0, len(unbound))
		for n := range unbound {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			v.addViolation("MDL-LISTOP01", linter.SeverityError,
				fmt.Sprintf("%s($%s, …): '$%s' is not defined in this microflow. A %s predicate is "+
					"evaluated once per item and Mendix binds the item to '$currentObject' — no other "+
					"iterator name exists, so mxbuild rejects this with CE0109 \"Undefined variable '%s'\".",
					op, lo.InputVariable, name, op, name),
				fmt.Sprintf("Use '$currentObject/…' for the item being tested, e.g. "+
					"%s($%s, $currentObject/Attribute > 0). A bare attribute name works too — mxcli "+
					"qualifies it against the list's entity.", op, lo.InputVariable))
		}
	})
}

// predicatePathVariables returns the base variable of every path navigation in a
// predicate ($x/Attr → "x"), deduplicated by the caller.
func predicatePathVariables(expr ast.Expression) []string {
	var out []string
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch n := e.(type) {
		case nil:
			return
		case *ast.AttributePathExpr:
			if n.Variable != "" {
				out = append(out, n.Variable)
			}
		case *ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Operand)
		case *ast.ParenExpr:
			walk(n.Inner)
		case *ast.FunctionCallExpr:
			for _, a := range n.Arguments {
				walk(a)
			}
		case *ast.IfThenElseExpr:
			walk(n.Condition)
			walk(n.ThenExpr)
			walk(n.ElseExpr)
		case *ast.SourceExpr:
			walk(n.Expression)
		}
	}
	walk(expr)
	return out
}

// forEachMicroflowStatement visits every statement in a body, descending into
// the nested bodies of loops, branches and splits.
func forEachMicroflowStatement(body []ast.MicroflowStatement, fn func(ast.MicroflowStatement)) {
	for _, s := range body {
		fn(s)
		switch st := s.(type) {
		case *ast.LoopStmt:
			forEachMicroflowStatement(st.Body, fn)
		case *ast.WhileStmt:
			forEachMicroflowStatement(st.Body, fn)
		case *ast.IfStmt:
			forEachMicroflowStatement(st.ThenBody, fn)
			forEachMicroflowStatement(st.ElseBody, fn)
		case *ast.EnumSplitStmt:
			for _, c := range st.Cases {
				forEachMicroflowStatement(c.Body, fn)
			}
			forEachMicroflowStatement(st.ElseBody, fn)
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				forEachMicroflowStatement(c.Body, fn)
			}
			forEachMicroflowStatement(st.ElseBody, fn)
		}
	}
}
