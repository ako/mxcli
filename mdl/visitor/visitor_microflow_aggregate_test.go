// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Making SET optional put `$X = <expr>` in front of every
// `VARIABLE EQUALS <function-call>` statement in the grammar, so
// `$Sum = sum($List.Price)` stopped reaching aggregateListStatement and fell
// through to the SET conversion instead — which joined the list and the
// attribute into one name. mxbuild rejected the result:
//
//	[CE0109] "Undefined variable 'ProductList.Price'."
//	[CE0015] "Aggregate function must specify a valid attribute."
//
// Both spellings must produce the same aggregate, whichever rule claims them.
func TestAggregateSplitsListFromAttribute(t *testing.T) {
	cases := []struct {
		name, src string
		wantOp    ast.AggregateListOperationType
		wantAttr  string
	}{
		{"bare sum", "$T = sum($ProductList.Price);", ast.AggregateSum, "Price"},
		{"bare average", "$T = average($ProductList.Price);", ast.AggregateAverage, "Price"},
		{"bare minimum", "$T = minimum($ProductList.Price);", ast.AggregateMinimum, "Price"},
		{"bare maximum", "$T = maximum($ProductList.Price);", ast.AggregateMaximum, "Price"},
		// The SET keyword routes through a different conversion; it was wrong
		// there before the bare form ever reached it.
		{"set sum", "set $T = sum($ProductList.Price);", ast.AggregateSum, "Price"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSingleAggregate(t, tc.src)
			if got.Operation != tc.wantOp {
				t.Errorf("operation = %v, want %v", got.Operation, tc.wantOp)
			}
			if got.InputVariable != "ProductList" {
				t.Errorf("input variable = %q, want ProductList (the list, without the attribute)", got.InputVariable)
			}
			if got.Attribute != tc.wantAttr {
				t.Errorf("attribute = %q, want %q", got.Attribute, tc.wantAttr)
			}
		})
	}
}

// `sum($List, <expression>)` aggregates a value computed per item. Losing the
// expression leaves an aggregate with nothing to aggregate — CE0015.
func TestAggregateKeepsPerItemExpression(t *testing.T) {
	for _, src := range []string{
		"$T = sum($ProductList, $currentObject/Price * 0.21);",
		"set $T = sum($ProductList, $currentObject/Price * 0.21);",
	} {
		got := parseSingleAggregate(t, src)
		if got.InputVariable != "ProductList" {
			t.Errorf("%s: input variable = %q, want ProductList", src, got.InputVariable)
		}
		if !got.IsExpression || got.Expression == nil {
			t.Errorf("%s: expression dropped (IsExpression=%v, Expression=%v)", src, got.IsExpression, got.Expression)
		}
	}
}

// COUNT takes the list alone and must not acquire an attribute.
func TestAggregateCountTakesTheListAlone(t *testing.T) {
	got := parseSingleAggregate(t, "$N = count($ProductList);")
	if got.Operation != ast.AggregateCount || got.InputVariable != "ProductList" || got.Attribute != "" {
		t.Errorf("got %+v, want COUNT over ProductList with no attribute", got)
	}
}

func parseSingleAggregate(t *testing.T, stmt string) *ast.AggregateListStmt {
	t.Helper()
	src := "create microflow M.A ($ProductList: list of M.Product)\nbegin\n  " + stmt + "\nend;"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", stmt, errs)
	}
	for _, s := range prog.Statements {
		cm, ok := s.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, st := range cm.Body {
			if agg, ok := st.(*ast.AggregateListStmt); ok {
				return agg
			}
			// A SET means the statement was swallowed as a plain value
			// assignment — that is the regression, and it reads better as a
			// failure here than as a nil dereference below.
			if set, ok := st.(*ast.MfSetStmt); ok {
				t.Fatalf("%q produced a Change Variable (target %q), not an aggregate", stmt, set.Target)
			}
		}
	}
	t.Fatalf("no AggregateListStmt produced by %q", stmt)
	return nil
}

// Mendix has eight aggregate functions; the grammar had five. DESCRIBE renders
// an activity by name, so a stored Reduce/All/Any came back out as MDL the
// parser could not read: `reduce(...)` fell through to a plain Change Variable
// and MDL044 then reported `reduce()` as "not a Mendix expression function"
// (#1004). Each of the three must reach an aggregate, not a SET.
func TestAggregateAcceptsReduceAllAny(t *testing.T) {
	cases := []struct {
		name, src  string
		wantOp     ast.AggregateListOperationType
		wantInit   bool
		wantReturn ast.DataTypeKind
	}{
		{
			name:       "reduce carries its seed and result type",
			src:        "$T = reduce($ProductList, $currentResult + $currentObject/Price, initial: 0, returns: Decimal);",
			wantOp:     ast.AggregateReduce,
			wantInit:   true,
			wantReturn: ast.TypeDecimal,
		},
		{
			name:   "all takes a bare boolean expression",
			src:    "$T = all($ProductList, $currentObject/Price > 0);",
			wantOp: ast.AggregateAll,
		},
		{
			name:   "any takes a bare boolean expression",
			src:    "$T = any($ProductList, $currentObject/Price > 0);",
			wantOp: ast.AggregateAny,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSingleAggregate(t, tc.src)
			if got.Operation != tc.wantOp {
				t.Errorf("operation = %v, want %v", got.Operation, tc.wantOp)
			}
			if got.InputVariable != "ProductList" {
				t.Errorf("input variable = %q, want %q", got.InputVariable, "ProductList")
			}
			if !got.IsExpression || got.Expression == nil {
				t.Errorf("expression form not recognised: IsExpression=%v Expression=%v", got.IsExpression, got.Expression)
			}
			if tc.wantInit {
				if got.InitialValue == nil {
					t.Error("initial value missing — reduce would be stored with an empty fold")
				}
				if got.ReturnType == nil {
					t.Fatal("return type missing — reduce would be stored with no result type")
				}
				if got.ReturnType.Kind != tc.wantReturn {
					t.Errorf("return type kind = %v, want %v", got.ReturnType.Kind, tc.wantReturn)
				}
			} else if got.InitialValue != nil || got.ReturnType != nil {
				// ALL/ANY have no seed and always fold to Boolean; writing either
				// from MDL would let an author contradict Mendix.
				t.Errorf("%v should carry no seed or declared type, got initial=%v returns=%v",
					tc.wantOp, got.InitialValue, got.ReturnType)
			}
		})
	}
}

// Adding REDUCE, ANY and INITIAL as lexer tokens takes those words out of
// circulation as bare identifiers unless they are also listed in the `keyword`
// rule. They are, and this is the check that says so.
func TestReduceKeywordsStayUsableAsIdentifiers(t *testing.T) {
	src := "create entity M.Thing (\n  reduce: string(10),\n  any: string(10),\n  initial: string(10)\n);"
	if _, errs := Build(src); len(errs) > 0 {
		t.Fatalf("new keywords are no longer usable as attribute names: %v", errs)
	}
}
