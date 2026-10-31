// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1832: `case-split: first-right-rest-columns` keeps a CASE's first
// outcome on the centre line and lays every other outcome out as a column under
// the split, side by side, in case order.

func caseColumnsLayout() *flowLayout { return &flowLayout{CaseSplit: CaseSplitColumns} }

func enumCase(value string, body []ast.MicroflowStatement) ast.EnumSplitCase {
	return ast.EnumSplitCase{Value: value, Body: body}
}

func enumSplit(cases []ast.EnumSplitCase, elseBody []ast.MicroflowStatement) *ast.EnumSplitStmt {
	return &ast.EnumSplitStmt{Variable: "Status", Cases: cases, ElseBody: elseBody}
}

func TestCaseSplitColumns_FirstOnTheLineRestAsColumns(t *testing.T) {
	s := enumSplit([]ast.EnumSplitCase{
		enumCase("Open", logStatements(2)),
		enumCase("Closed", logStatements(2)),
		enumCase("Archived", logStatements(3)),
	}, logStatements(1))
	stmts := append([]ast.MicroflowStatement{s}, logStatements(1)...)
	fb := buildWithLayout(t, stmts, caseColumnsLayout())

	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	merge, ok := firstOfType[*microflows.ExclusiveMerge](fb.objects)
	if !ok {
		t.Fatal("no merge")
	}
	sp, mp := split.GetPosition(), merge.GetPosition()
	acts := activityPositions(fb.objects)
	if len(acts) != 9 {
		t.Fatalf("want 9 activities, got %d", len(acts))
	}
	first, closed, archived, other, after := acts[0:2], acts[2:4], acts[4:7], acts[7:8], acts[8]

	for _, p := range first {
		if p.Y != sp.Y {
			t.Errorf("first case at %v, want on the centre line y=%d", p, sp.Y)
		}
	}
	columns := [][]model.Point{closed, archived, other}
	prevX := 0
	for c, col := range columns {
		for i, p := range col {
			if p.X != col[0].X {
				t.Errorf("column %d statement %d at x=%d, want the column's x=%d", c, i, p.X, col[0].X)
			}
			if i > 0 && p.Y <= col[i-1].Y {
				t.Errorf("column %d statement %d at y=%d, not under the one before at y=%d", c, i, p.Y, col[i-1].Y)
			}
		}
		if col[0].Y <= sp.Y {
			t.Errorf("column %d starts at y=%d, not under the split at y=%d", c, col[0].Y, sp.Y)
		}
		if c == 0 && col[0].X != sp.X {
			t.Errorf("second case's column at x=%d, want under the split x=%d", col[0].X, sp.X)
		}
		if c > 0 && col[0].X <= prevX {
			t.Errorf("column %d at x=%d is not right of column %d at x=%d", c, col[0].X, c-1, prevX)
		}
		prevX = col[0].X
	}
	if mp.Y != sp.Y {
		t.Errorf("merge at %v, want on the centre line", mp)
	}
	if after.Y != sp.Y || after.X <= mp.X {
		t.Errorf("activity after the CASE at %v, want on the centre line past the merge at %v", after, mp)
	}

	// The first case leaves the right; every other leaves the bottom into its
	// column's top, and each column rejoins the merge from below.
	for _, f := range fb.flows {
		if f.OriginID != split.GetID() {
			continue
		}
		dest := objectByIDIn(fb.objects, f.DestinationID).GetPosition()
		if dest.Y == sp.Y {
			if f.OriginConnectionIndex != AnchorRight || f.DestinationConnectionIndex != AnchorLeft {
				t.Errorf("first case flow sides %d->%d, want right->left", f.OriginConnectionIndex, f.DestinationConnectionIndex)
			}
		} else if f.OriginConnectionIndex != AnchorBottom || f.DestinationConnectionIndex != AnchorTop {
			t.Errorf("column flow to %v sides %d->%d, want bottom->top", dest, f.OriginConnectionIndex, f.DestinationConnectionIndex)
		}
	}
	assertNoOverlap(t, fb.objects)
}

func TestCaseSplitColumns_DefaultLayoutIsUnchanged(t *testing.T) {
	s := enumSplit([]ast.EnumSplitCase{
		enumCase("Open", logStatements(2)),
		enumCase("Closed", logStatements(2)),
	}, nil)
	want := buildRows(t, []ast.MicroflowStatement{s})
	fb := buildWithLayout(t, []ast.MicroflowStatement{s}, &flowLayout{})
	for i, o := range fb.objects {
		if o.GetPosition() != want[i].GetPosition() {
			t.Fatalf("object %d at %v, default builder puts it at %v", i, o.GetPosition(), want[i].GetPosition())
		}
	}
}

func TestCaseSplitColumns_MultiValueCaseAndNesting(t *testing.T) {
	inner := ifElse(logStatements(1), logStatements(2))
	s := enumSplit([]ast.EnumSplitCase{
		{Values: []string{"Open", "New"}, Body: logStatements(1)},
		{Values: []string{"Closed", "Done"}, Body: []ast.MicroflowStatement{inner, logStatements(1)[0]}},
		enumCase("Archived", append(logStatements(1), &ast.ReturnStmt{})),
	}, nil)
	stmts := append([]ast.MicroflowStatement{s}, logStatements(1)...)
	for _, l := range []*flowLayout{caseColumnsLayout(), {CaseSplit: CaseSplitColumns, IfElse: IfElseColumn}} {
		fb := buildWithLayout(t, stmts, l)
		assertNoOverlap(t, fb.objects)
	}
}

func TestCaseSplitColumns_InheritanceSplit(t *testing.T) {
	s := &ast.InheritanceSplitStmt{
		Variable: "Animal",
		Cases: []ast.InheritanceSplitCase{
			{Entity: ast.QualifiedName{Module: "M", Name: "Dog"}, Body: logStatements(2)},
			{Entity: ast.QualifiedName{Module: "M", Name: "Cat"}, Body: logStatements(2)},
			{Entity: ast.QualifiedName{Module: "M", Name: "Bird"}, Body: logStatements(1)},
		},
	}
	stmts := append([]ast.MicroflowStatement{s}, logStatements(1)...)
	fb := buildWithLayout(t, stmts, caseColumnsLayout())
	split, _ := firstOfType[*microflows.InheritanceSplit](fb.objects)
	sp := split.GetPosition()
	acts := activityPositions(fb.objects)
	if acts[0].Y != sp.Y || acts[1].Y != sp.Y {
		t.Errorf("first case %v, want on the centre line y=%d", acts[0:2], sp.Y)
	}
	if acts[2].X != sp.X || acts[3].X != sp.X || acts[3].Y <= acts[2].Y {
		t.Errorf("second case %v, want a column under the split x=%d", acts[2:4], sp.X)
	}
	if acts[4].X <= sp.X || acts[4].Y != acts[2].Y {
		t.Errorf("third case %v, want a column right of the second, level with it", acts[4])
	}
	assertNoOverlap(t, fb.objects)
}

func TestIfElseColumn_NestedColumnInTheTrueBodyIsCleared(t *testing.T) {
	// The TRUE body holds an IF whose ELSE column hangs below the line: the outer
	// ELSE column must start below it. The measurer reports the inner IF's height
	// as if centred on the line, so this takes it doubling what reaches below.
	// The outer ELSE holds an IF of its own whose TRUE body runs right, under the
	// inner column, so starting the outer column too high puts the two on top of
	// each other.
	inner := ifElse(logStatements(1), logStatements(3))
	outerElse := []ast.MicroflowStatement{ifElse(logStatements(3), logStatements(1))}
	stmts := []ast.MicroflowStatement{ifElse([]ast.MicroflowStatement{inner}, outerElse)}
	fb := buildWithLayout(t, stmts, columnLayout())
	assertNoOverlap(t, fb.objects)
}

// The stored flow order does not survive serialization, so DESCRIBE reads a
// CASE's order back from the drawing. Columns share one side pair and one top,
// so in a flow recording case-split columns the order is finished left to right.
func TestCaseSplitColumns_DescribeReadsColumnsLeftToRight(t *testing.T) {
	cases := []ast.EnumSplitCase{enumCase("V1", logStatements(1))}
	for i := 2; i <= 5; i++ {
		cases = append(cases, enumCase(fmt.Sprintf("V%d", i), logStatements(i%3+1)))
	}
	fb := buildWithLayout(t, []ast.MicroflowStatement{enumSplit(cases, nil)}, caseColumnsLayout())
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range fb.objects {
		objects[o.GetID()] = o
	}
	var flows []*microflows.SequenceFlow
	for _, f := range fb.flows {
		if f.OriginID == split.GetID() {
			flows = append(flows, f)
		}
	}
	ctx := &ExecContext{describedFlowLayout: caseColumnsLayout()}
	for seed := int64(1); seed <= 5; seed++ {
		rand.New(rand.NewSource(seed)).Shuffle(len(flows), func(i, j int) { flows[i], flows[j] = flows[j], flows[i] })
		for i, f := range orderedEnumSplitFlowsFor(ctx, flows, objects) {
			if v, _ := enumCaseValue(f); v != fmt.Sprintf("V%d", i+1) {
				t.Fatalf("seed %d: position %d holds %s", seed, i, v)
			}
		}
	}
}

func TestCaseSplitColumns_DescribeReadsInheritanceColumnsLeftToRight(t *testing.T) {
	s := &ast.InheritanceSplitStmt{Variable: "Animal"}
	names := []string{"Dog", "Cat", "Bird", "Fish"}
	for i, n := range names {
		s.Cases = append(s.Cases, ast.InheritanceSplitCase{Entity: ast.QualifiedName{Module: "M", Name: n}, Body: logStatements(i%2 + 1)})
	}
	fb := buildWithLayout(t, []ast.MicroflowStatement{s}, caseColumnsLayout())
	split, _ := firstOfType[*microflows.InheritanceSplit](fb.objects)
	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range fb.objects {
		objects[o.GetID()] = o
	}
	var flows []*microflows.SequenceFlow
	for _, f := range fb.flows {
		if f.OriginID == split.GetID() {
			if _, isMerge := objects[f.DestinationID].(*microflows.ExclusiveMerge); !isMerge {
				flows = append(flows, f) // the (empty) flow straight to the merge orders itself by its pair
			}
		}
	}
	ctx := &ExecContext{describedFlowLayout: caseColumnsLayout()}
	for seed := int64(1); seed <= 5; seed++ {
		rand.New(rand.NewSource(seed)).Shuffle(len(flows), func(i, j int) { flows[i], flows[j] = flows[j], flows[i] })
		for i, f := range orderedInheritanceSplitFlowsFor(ctx, flows, objects) {
			ic, _ := f.CaseValue.(*microflows.InheritanceCase)
			if ic == nil || ic.EntityQualifiedName != "M."+names[i] {
				t.Fatalf("seed %d: position %d holds %+v", seed, i, f.CaseValue)
			}
		}
	}
}
