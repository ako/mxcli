// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// listopIteratorViolations runs the microflow validator and returns only the
// MDL-LISTOP01 messages.
func listopIteratorViolations(t *testing.T, stmt *ast.CreateMicroflowStmt) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL-LISTOP01" {
			out = append(out, v.Message)
		}
	}
	return out
}

// pathPredicate builds `$<variable>/<attr> > 0`.
func pathPredicate(variable, attr string) ast.Expression {
	return &ast.BinaryExpr{
		Left: &ast.AttributePathExpr{
			Variable: variable,
			Path:     []string{attr},
			Segments: []ast.PathSegment{{Name: attr, Separator: "/"}},
		},
		Operator: ">",
		Right:    &ast.LiteralExpr{Value: "0", Kind: ast.LiteralInteger},
	}
}

func filterStmt(input string, cond ast.Expression) *ast.ListOperationStmt {
	return &ast.ListOperationStmt{
		Operation:      ast.ListOpFilter,
		InputVariable:  input,
		OutputVariable: "R",
		Condition:      cond,
	}
}

// TestListOperationIteratorUndefinedVariable is issue #1002's CE0109 half.
// Mendix binds the item to $currentObject and defines no other iterator, so
// `filter($L, $item/Qty > 0)` fails the build — which mxcli used to pass
// through silently.
func TestListOperationIteratorUndefinedVariable(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "BadIterator"},
		Body: []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "L"},
			filterStmt("L", pathPredicate("item", "Qty")),
		},
	}
	got := listopIteratorViolations(t, stmt)
	if len(got) != 1 {
		t.Fatalf("expected 1 MDL-LISTOP01 violation, got %d: %v", len(got), got)
	}
	for _, want := range []string{"$item", "$currentObject", "CE0109"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message %q does not mention %q", got[0], want)
		}
	}
}

// TestListOperationIteratorAcceptsCurrentObject is the control that stops the
// rule from firing on the correct spelling.
func TestListOperationIteratorAcceptsCurrentObject(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Good"},
		Body: []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "L"},
			filterStmt("L", pathPredicate("currentObject", "Qty")),
		},
	}
	if got := listopIteratorViolations(t, stmt); len(got) != 0 {
		t.Errorf("$currentObject predicate flagged: %v", got)
	}
}

// TestListOperationIteratorAcceptsEnclosingLoopVariable is the control that
// matters most. The rule keys on SCOPE, not on the name: '$item' is valid in a
// predicate when it is the enclosing loop's iterator, which is exactly how
// CLAUDE.md's O(N) lookup idiom is written —
// `find($TargetList, key = $item/key)` inside `loop $item in $L`. Keying on the
// name would break the documented pattern; this microflow builds at 0 errors on
// mxbuild 11.13.0.
func TestListOperationIteratorAcceptsEnclosingLoopVariable(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name:       ast.QualifiedName{Module: "Shop", Name: "LoopLookup"},
		Parameters: []ast.MicroflowParam{{Name: "Others"}},
		Body: []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "L"},
			&ast.LoopStmt{
				LoopVariable: "item",
				ListVariable: "L",
				Body: []ast.MicroflowStatement{
					filterStmt("Others", pathPredicate("item", "Qty")),
				},
			},
		},
	}
	if got := listopIteratorViolations(t, stmt); len(got) != 0 {
		t.Errorf("the enclosing loop's iterator was flagged: %v", got)
	}
}

// TestListOperationIteratorAcceptsParameter: a microflow parameter is in scope
// in a predicate just like any other variable.
func TestListOperationIteratorAcceptsParameter(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name:       ast.QualifiedName{Module: "Shop", Name: "ParamRef"},
		Parameters: []ast.MicroflowParam{{Name: "Wanted"}},
		Body: []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "L"},
			filterStmt("L", pathPredicate("Wanted", "Qty")),
		},
	}
	if got := listopIteratorViolations(t, stmt); len(got) != 0 {
		t.Errorf("a parameter reference was flagged: %v", got)
	}
}

// TestListOperationIteratorIgnoresSort: SORT carries no predicate expression
// (it takes attribute names), so the rule must not reach into it.
func TestListOperationIteratorIgnoresSort(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Sorted"},
		Body: []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "L"},
			&ast.ListOperationStmt{
				Operation:      ast.ListOpSort,
				InputVariable:  "L",
				OutputVariable: "R",
				SortSpecs:      []ast.SortSpec{{Attribute: "Qty", Ascending: false}},
			},
		},
	}
	if got := listopIteratorViolations(t, stmt); len(got) != 0 {
		t.Errorf("SORT was flagged: %v", got)
	}
}
