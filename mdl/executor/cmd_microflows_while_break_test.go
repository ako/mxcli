// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #893 case 3. `while $C begin if X then break; end if; end while;` built an
// ExclusiveSplit carrying ONLY its `true` outgoing flow (→ BreakEvent). The
// `false` case was deferred to a following statement that never came, so the
// decision shipped with no false flow — CE0079 "the 'false' condition value
// should be configured on an outgoing sequence flow".
//
// The identical body under LOOP has been correct since ledger #52: addLoopStatement
// tracks the deferred case through the body and wires whatever is left over to a
// synthesized ContinueEvent ("didn't break → next iteration"). addWhileStatement
// never mirrored either half, so the two loop builders disagreed on the same body.
//
// This is a writer defect, not a check gap: `mxcli check` reported "Syntax OK"
// and exit 0, and `exec` then wrote the broken microflow.
func TestBuilder_ConditionalBreakLastInWhile_HasFalseFlow(t *testing.T) {
	col := buildWhileBody(t, []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody:  []ast.MicroflowStatement{&ast.BreakStmt{}},
		},
	})

	splitID, hasBreak, hasContinue := "", false, false
	for _, o := range col.Objects {
		loop, ok := o.(*microflows.LoopedActivity)
		if !ok {
			continue
		}
		for _, inner := range loop.ObjectCollection.Objects {
			switch v := inner.(type) {
			case *microflows.ExclusiveSplit:
				splitID = string(v.ID)
			case *microflows.BreakEvent:
				hasBreak = true
			case *microflows.ContinueEvent:
				hasContinue = true
			}
		}
	}
	if splitID == "" {
		t.Fatal("no ExclusiveSplit found in the while body")
	}
	if !hasBreak {
		t.Error("expected a BreakEvent in the while body")
	}
	if !hasContinue {
		t.Error("expected a synthesized ContinueEvent for the split's false branch")
	}

	trueCount, falseCount := countSplitCases(col, splitID)
	if trueCount != 1 {
		t.Errorf("split should have exactly 1 true flow (→ break), got %d", trueCount)
	}
	if falseCount != 1 {
		t.Errorf("split should have exactly 1 false flow (→ continue), got %d — a decision with no false flow is CE0079", falseCount)
	}
}

// The two loop builders must agree: the same merge-less-split body is either
// correct under both LOOP and WHILE or broken under both. Asserting parity is
// what keeps a future fix to one from silently skipping the other, which is the
// shape of this defect.
func TestBuilder_MergelessSplitFalseFlow_LoopAndWhileAgree(t *testing.T) {
	ifBreak := func() ast.MicroflowStatement {
		return &ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody:  []ast.MicroflowStatement{&ast.BreakStmt{}},
		}
	}

	loopCol := buildStatements(t, &ast.LoopStmt{
		ListVariable: "L", LoopVariable: "R",
		Body: []ast.MicroflowStatement{ifBreak()},
	}, map[string]string{"L": "List of M.R"})
	whileCol := buildStatements(t, &ast.WhileStmt{
		Condition: &ast.VariableExpr{Name: "C"},
		Body:      []ast.MicroflowStatement{ifBreak()},
	}, map[string]string{})

	loopTrue, loopFalse := countSplitCases(loopCol, firstSplitID(loopCol))
	whileTrue, whileFalse := countSplitCases(whileCol, firstSplitID(whileCol))

	if loopTrue != whileTrue || loopFalse != whileFalse {
		t.Errorf("loop and while disagree on the same body: loop true=%d false=%d, while true=%d false=%d",
			loopTrue, loopFalse, whileTrue, whileFalse)
	}
	if whileFalse != 1 {
		t.Errorf("while split has %d false flows, want 1 (CE0079)", whileFalse)
	}
}

// A merge-less split mid-body must hand its deferred false case to the NEXT
// statement rather than dropping it — the same bookkeeping, one statement earlier.
func TestBuilder_MergelessSplitMidWhileBody_FalseFlowReachesNextStatement(t *testing.T) {
	col := buildWhileBody(t, []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody:  []ast.MicroflowStatement{&ast.BreakStmt{}},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "after"}},
	})

	splitID := firstSplitID(col)
	if splitID == "" {
		t.Fatal("no ExclusiveSplit found in the while body")
	}
	if _, falseCount := countSplitCases(col, splitID); falseCount != 1 {
		t.Errorf("split has %d false flows, want 1 — the deferred case was dropped (CE0079)", falseCount)
	}
}

// --- helpers ---------------------------------------------------------------

func buildWhileBody(t *testing.T, body []ast.MicroflowStatement) *microflows.MicroflowObjectCollection {
	t.Helper()
	return buildStatements(t, &ast.WhileStmt{
		Condition: &ast.VariableExpr{Name: "C"},
		Body:      body,
	}, map[string]string{})
}

func buildStatements(t *testing.T, stmt ast.MicroflowStatement, vars map[string]string) *microflows.MicroflowObjectCollection {
	t.Helper()
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
		varTypes: vars,
	}
	return fb.buildFlowGraph([]ast.MicroflowStatement{stmt}, nil)
}

// firstSplitID returns the ID of the first ExclusiveSplit anywhere in the graph,
// including inside a LoopedActivity's body.
func firstSplitID(col *microflows.MicroflowObjectCollection) string {
	var walk func(objs []microflows.MicroflowObject) string
	walk = func(objs []microflows.MicroflowObject) string {
		for _, o := range objs {
			switch v := o.(type) {
			case *microflows.ExclusiveSplit:
				return string(v.ID)
			case *microflows.LoopedActivity:
				if id := walk(v.ObjectCollection.Objects); id != "" {
					return id
				}
			}
		}
		return ""
	}
	return walk(col.Objects)
}

// countSplitCases counts the true/false labelled flows leaving splitID. A loop
// body's flows are lifted to the top-level collection, so both levels are scanned.
func countSplitCases(col *microflows.MicroflowObjectCollection, splitID string) (int, int) {
	trueCount, falseCount := 0, 0
	bump := func(v string) {
		switch v {
		case "true":
			trueCount++
		case "false":
			falseCount++
		}
	}
	scan := func(flows []*microflows.SequenceFlow) {
		for _, f := range flows {
			if string(f.OriginID) != splitID {
				continue
			}
			switch cv := f.CaseValue.(type) {
			case *microflows.ExpressionCase:
				bump(cv.Expression)
			case microflows.ExpressionCase:
				bump(cv.Expression)
			case *microflows.EnumerationCase:
				bump(cv.Value)
			case microflows.EnumerationCase:
				bump(cv.Value)
			}
		}
	}
	scan(col.Flows)
	var walk func(objs []microflows.MicroflowObject)
	walk = func(objs []microflows.MicroflowObject) {
		for _, o := range objs {
			if loop, ok := o.(*microflows.LoopedActivity); ok {
				scan(loop.ObjectCollection.Flows)
				walk(loop.ObjectCollection.Objects)
			}
		}
	}
	walk(col.Objects)
	return trueCount, falseCount
}
