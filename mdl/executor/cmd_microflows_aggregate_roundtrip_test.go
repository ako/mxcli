// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// aggregateOperationName maps the AST's aggregate kind back to the Mendix
// function it builds, so the round trip below can compare like with like.
var aggregateOperationName = map[ast.AggregateListOperationType]microflows.AggregateFunction{
	ast.AggregateCount:   microflows.AggregateFunctionCount,
	ast.AggregateSum:     microflows.AggregateFunctionSum,
	ast.AggregateAverage: microflows.AggregateFunctionAverage,
	ast.AggregateMinimum: microflows.AggregateFunctionMin,
	ast.AggregateMaximum: microflows.AggregateFunctionMax,
	ast.AggregateReduce:  microflows.AggregateFunctionReduce,
	ast.AggregateAll:     microflows.AggregateFunctionAll,
	ast.AggregateAny:     microflows.AggregateFunctionAny,
}

// TestDescribedAggregateParsesBack is the test #1004 needed and did not have.
//
// DESCRIBE rendered an aggregate by lowercasing whatever Mendix had stored,
// which silently assumed every value of the AggregateFunction enumeration was
// also an MDL keyword. Five were; Reduce, All and Any were not, so a Studio
// Pro-authored activity described as MDL that mxcli's own checker rejected
// ("'reduce()' ... is not a Mendix expression function [MDL044]").
//
// Driving every function through describe and back through the parser is what
// makes that class of gap impossible to reintroduce: a ninth function added to
// Mendix fails here rather than in a user's script.
func TestDescribedAggregateParsesBack(t *testing.T) {
	for _, fn := range microflows.AllAggregateFunctions {
		t.Run(string(fn), func(t *testing.T) {
			action := &microflows.AggregateListAction{
				InputVariable:  "ProductList",
				OutputVariable: "Result",
				Function:       fn,
				UseExpression:  true,
				Expression:     "$currentObject/Price",
			}
			// Reduce is the one function whose fold is not derivable, so a
			// describe that omits it is lossy even if it parses.
			if fn == microflows.AggregateFunctionReduce {
				action.ReduceInitialValue = "0"
				action.ReduceReturnType = &microflows.DecimalType{}
			}

			rendered := formatAction(nil, action, nil, nil)
			if strings.HasPrefix(strings.TrimSpace(rendered), "//") {
				t.Fatalf("%s has no MDL keyword — DESCRIBE cannot render it: %s", fn, rendered)
			}

			src := "create microflow M.A ($ProductList: list of M.Product)\nbegin\n  " + rendered + "\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted MDL the parser rejects.\n  rendered: %s\n  errors:   %v", rendered, errs)
			}

			agg := findAggregate(t, prog, rendered)
			if got := aggregateOperationName[agg.Operation]; got != fn {
				t.Errorf("round trip changed the function: %s -> %s (rendered %q)", fn, got, rendered)
			}
			if fn == microflows.AggregateFunctionReduce {
				if agg.InitialValue == nil || agg.ReturnType == nil {
					t.Errorf("reduce lost its fold on the way back: initial=%v returns=%v (rendered %q)",
						agg.InitialValue, agg.ReturnType, rendered)
				}
			}
		})
	}
}

// TestAggregateKeywordsCoverEveryFunction guards the mapping itself, so a
// function added to microflows.AllAggregateFunctions without an MDL keyword
// fails here with a clear reason rather than as a parse error above.
func TestAggregateKeywordsCoverEveryFunction(t *testing.T) {
	for _, fn := range microflows.AllAggregateFunctions {
		if _, ok := mdlAggregateKeyword(fn); !ok {
			t.Errorf("aggregate function %q has no MDL keyword — DESCRIBE would refuse to render it", fn)
		}
	}
}

// findAggregate returns the single aggregate in a parsed microflow, failing
// with the statement kind that was produced instead. `reduce(...)` used to
// parse as a Change Variable, which is exactly how #1004 presented.
func findAggregate(t *testing.T, prog *ast.Program, rendered string) *ast.AggregateListStmt {
	t.Helper()
	for _, s := range prog.Statements {
		cm, ok := s.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, st := range cm.Body {
			if agg, ok := st.(*ast.AggregateListStmt); ok {
				return agg
			}
			if set, ok := st.(*ast.MfSetStmt); ok {
				t.Fatalf("%q parsed as a Change Variable of $%s, not an aggregate", rendered, set.Target)
			}
		}
	}
	t.Fatalf("no aggregate produced by %q", rendered)
	return nil
}
