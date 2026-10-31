// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1832: `if-else: true-right-else-column` keeps the TRUE body on the
// centre line and stacks the ELSE body top to bottom in a column under the split.

// buildWithLayout lays statements out the way buildRows does, under a style.
func buildWithLayout(t *testing.T, stmts []ast.MicroflowStatement, layout *flowLayout) *flowBuilder {
	t.Helper()
	fb := &flowBuilder{
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		allowWrap:    true,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{}},
		layout:       layout,
	}
	fb.buildFlowGraph(stmts, nil)
	return fb
}

func columnLayout() *flowLayout { return &flowLayout{IfElse: IfElseColumn} }

func ifElse(thenBody, elseBody []ast.MicroflowStatement) *ast.IfStmt {
	return &ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody:  thenBody,
		ElseBody:  elseBody,
		HasElse:   true,
	}
}

func objectByIDIn(objects []microflows.MicroflowObject, id model.ID) microflows.MicroflowObject {
	for _, o := range objects {
		if o.GetID() == id {
			return o
		}
	}
	return nil
}

func sizeOf(o microflows.MicroflowObject) model.Size {
	if ws, ok := o.(interface{ GetSize() model.Size }); ok {
		if s := ws.GetSize(); s.Width > 0 && s.Height > 0 {
			return s
		}
	}
	return model.Size{Width: ActivityWidth, Height: ActivityHeight}
}

// flowsWithCase returns the flows leaving origin with the given boolean case.
func flowFrom(flows []*microflows.SequenceFlow, origin model.ID, caseValue string) *microflows.SequenceFlow {
	for _, f := range flows {
		if f.OriginID != origin {
			continue
		}
		ec, ok := f.CaseValue.(*microflows.ExpressionCase)
		if caseValue == "" && f.CaseValue == nil {
			return f
		}
		if ok && ec.Expression == caseValue {
			return f
		}
	}
	return nil
}

func flowBetween(flows []*microflows.SequenceFlow, origin, dest model.ID) *microflows.SequenceFlow {
	for _, f := range flows {
		if f.OriginID == origin && f.DestinationID == dest {
			return f
		}
	}
	return nil
}

func firstOfType[T microflows.MicroflowObject](objects []microflows.MicroflowObject) (T, bool) {
	for _, o := range objects {
		if v, ok := o.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// assertNoOverlap fails when two placed objects' boxes intersect. A loop box
// contains its body, so an object inside a loop box is not an overlap; anything
// else inside another box is.
func assertNoOverlap(t *testing.T, objects []microflows.MicroflowObject) {
	t.Helper()
	type box struct{ l, t, r, b int }
	boxOf := func(o microflows.MicroflowObject) box {
		p, s := o.GetPosition(), sizeOf(o)
		return box{p.X - s.Width/2, p.Y - s.Height/2, p.X + s.Width/2, p.Y + s.Height/2}
	}
	contains := func(a, b box) bool { return a.l <= b.l && a.t <= b.t && a.r >= b.r && a.b >= b.b }
	for i := range objects {
		for j := i + 1; j < len(objects); j++ {
			a, b := boxOf(objects[i]), boxOf(objects[j])
			_, iLoop := objects[i].(*microflows.LoopedActivity)
			_, jLoop := objects[j].(*microflows.LoopedActivity)
			if (iLoop && contains(a, b)) || (jLoop && contains(b, a)) {
				continue
			}
			if a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b {
				t.Errorf("%T at %v overlaps %T at %v", objects[i], objects[i].GetPosition(), objects[j], objects[j].GetPosition())
			}
		}
	}
}

func TestIfElseColumn_ElseBodyIsAColumnUnderTheSplit(t *testing.T) {
	stmts := []ast.MicroflowStatement{ifElse(logStatements(1), logStatements(3))}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, columnLayout())

	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	merge, ok := firstOfType[*microflows.ExclusiveMerge](fb.objects)
	if !ok {
		t.Fatal("no merge")
	}
	acts := activityPositions(fb.objects)
	if len(acts) != 5 {
		t.Fatalf("want 5 activities, got %d", len(acts))
	}
	then, column, after := acts[0], acts[1:4], acts[4]
	sp := split.GetPosition()

	if then.Y != sp.Y {
		t.Errorf("TRUE body at y=%d, want the centre line y=%d", then.Y, sp.Y)
	}
	for i, p := range column {
		if p.X != sp.X {
			t.Errorf("ELSE statement %d at x=%d, want the split's column x=%d", i, p.X, sp.X)
		}
	}
	if top := column[0].Y - ActivityHeight/2; top < then.Y+ActivityHeight/2+BranchGap {
		t.Errorf("first ELSE statement's top y=%d is not clear of the TRUE body (bottom %d)", top, then.Y+ActivityHeight/2)
	}
	for i := 1; i < len(column); i++ {
		if gap := (column[i].Y - ActivityHeight/2) - (column[i-1].Y + ActivityHeight/2); gap != RowGap {
			t.Errorf("ELSE statements %d and %d are %d px apart edge to edge, want RowGap=%d", i-1, i, gap, RowGap)
		}
	}
	mp := merge.GetPosition()
	if mp.Y != sp.Y {
		t.Errorf("merge at y=%d, want the centre line y=%d", mp.Y, sp.Y)
	}
	if mp.X-MergeSize/2 <= column[0].X+ActivityWidth/2 || mp.X-MergeSize/2 <= then.X+ActivityWidth/2 {
		t.Errorf("merge at x=%d is not right of both branches", mp.X)
	}
	if after.Y != sp.Y || after.X <= mp.X {
		t.Errorf("activity after the IF at %v, want on the centre line past the merge at %v", after, mp)
	}

	// Exits: TRUE right, FALSE bottom into the top of the column; the column runs
	// bottom to top; its last element rejoins the merge from below.
	if f := flowFrom(fb.flows, split.GetID(), "true"); f == nil || f.OriginConnectionIndex != AnchorRight {
		t.Errorf("TRUE flow = %+v, want it to leave the split's right side", f)
	}
	f := flowFrom(fb.flows, split.GetID(), "false")
	if f == nil || f.OriginConnectionIndex != AnchorBottom || f.DestinationConnectionIndex != AnchorTop {
		t.Errorf("FALSE flow = %+v, want bottom of split -> top of the first ELSE statement", f)
	}
	ids := activityIDs(fb.objects)
	for i := 1; i < 3; i++ {
		f := flowBetween(fb.flows, ids[i], ids[i+1])
		if f == nil || f.OriginConnectionIndex != AnchorBottom || f.DestinationConnectionIndex != AnchorTop {
			t.Errorf("column flow %d = %+v, want bottom -> top", i, f)
		}
	}
	if f := flowBetween(fb.flows, ids[3], merge.GetID()); f == nil || f.OriginConnectionIndex != AnchorRight || f.DestinationConnectionIndex != AnchorBottom {
		t.Errorf("column -> merge flow = %+v, want right -> bottom", f)
	}
	assertNoOverlap(t, fb.objects)
}

func TestIfElseColumn_DefaultLayoutIsUnchanged(t *testing.T) {
	// Control: without the style the ELSE body is a horizontal lane, as before.
	stmts := []ast.MicroflowStatement{ifElse(logStatements(1), logStatements(3))}
	for _, layout := range []*flowLayout{nil, {}} {
		fb := buildWithLayout(t, stmts, layout)
		acts := activityPositions(fb.objects)
		if acts[1].Y != acts[2].Y || acts[2].Y != acts[3].Y || acts[1].X >= acts[2].X {
			t.Fatalf("layout %v: ELSE body %v is not a horizontal lane", layout, acts[1:])
		}
		want := buildRows(t, stmts)
		for i, o := range fb.objects {
			if o.GetPosition() != want[i].GetPosition() {
				t.Fatalf("layout %v: object %d at %v, default builder puts it at %v", layout, i, o.GetPosition(), want[i].GetPosition())
			}
		}
	}
}

func TestIfElseColumn_EmptyThenRunsStraightToTheMerge(t *testing.T) {
	fb := buildWithLayout(t, []ast.MicroflowStatement{ifElse(nil, logStatements(2))}, columnLayout())
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	merge, ok := firstOfType[*microflows.ExclusiveMerge](fb.objects)
	if !ok {
		t.Fatal("no merge")
	}
	if merge.GetPosition().Y != split.GetPosition().Y {
		t.Errorf("merge at %v is not on the centre line of the split at %v", merge.GetPosition(), split.GetPosition())
	}
	f := flowBetween(fb.flows, split.GetID(), merge.GetID())
	if f == nil || f.OriginConnectionIndex != AnchorRight || f.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("TRUE flow = %+v, want split right -> merge left", f)
	}
	assertNoOverlap(t, fb.objects)
}

func TestIfElseColumn_ElseReturnEndsAtTheBottomOfTheColumn(t *testing.T) {
	elseBody := append(logStatements(2), &ast.ReturnStmt{})
	fb := buildWithLayout(t, []ast.MicroflowStatement{ifElse(logStatements(1), elseBody)}, columnLayout())
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	acts := activityPositions(fb.objects)
	last := acts[len(acts)-1]
	var end model.Point
	found := false
	for _, o := range fb.objects {
		if e, ok := o.(*microflows.EndEvent); ok && e.GetPosition().Y > split.GetPosition().Y {
			end, found = e.GetPosition(), true
		}
	}
	if !found {
		t.Fatal("no end event under the centre line")
	}
	if end.X != split.GetPosition().X || end.Y <= last.Y {
		t.Errorf("ELSE's end event at %v, want in the column under the last ELSE activity at %v", end, last)
	}
	assertNoOverlap(t, fb.objects)
}

func TestIfElseColumn_NestedIfInTheColumnDoesNotOverlap(t *testing.T) {
	inner := ifElse(logStatements(2), logStatements(2))
	elseBody := []ast.MicroflowStatement{inner}
	elseBody = append(elseBody, logStatements(2)...)
	stmts := []ast.MicroflowStatement{ifElse(logStatements(1), elseBody)}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, columnLayout())
	assertNoOverlap(t, fb.objects)

	// The outer merge clears the inner IF, which reaches right of the column.
	var splits []microflows.MicroflowObject
	var merges []microflows.MicroflowObject
	for _, o := range fb.objects {
		switch o.(type) {
		case *microflows.ExclusiveSplit:
			splits = append(splits, o)
		case *microflows.ExclusiveMerge:
			merges = append(merges, o)
		}
	}
	if len(splits) != 2 || len(merges) != 2 {
		t.Fatalf("want 2 splits and 2 merges, got %d and %d", len(splits), len(merges))
	}
	outerMerge := merges[0]
	for _, o := range fb.objects {
		if o == outerMerge {
			continue
		}
		if o.GetPosition().Y > splits[0].GetPosition().Y && o.GetPosition().X+sizeOf(o).Width/2 >= outerMerge.GetPosition().X {
			t.Errorf("%T at %v in the column reaches past the outer merge at %v", o, o.GetPosition(), outerMerge.GetPosition())
		}
	}
}

func activityIDs(objects []microflows.MicroflowObject) []model.ID {
	var out []model.ID
	for _, o := range objects {
		if _, ok := o.(*microflows.ActionActivity); ok {
			out = append(out, o.GetID())
		}
	}
	return out
}

func TestIfElseColumn_LoopAndGuardInTheColumnDoNotOverlap(t *testing.T) {
	loop := &ast.LoopStmt{LoopVariable: "Item", ListVariable: "Items", Body: logStatements(2)}
	elseBody := []ast.MicroflowStatement{loop, guardStmt()}
	elseBody = append(elseBody, logStatements(1)...)
	inner := ifElse(logStatements(1), logStatements(2)) // a column inside the column
	elseBody = append(elseBody, inner)
	stmts := []ast.MicroflowStatement{ifElse(logStatements(2), elseBody)}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, columnLayout())
	assertNoOverlap(t, fb.objects)

	// Every statement of the column starts under the previous one.
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	loopObj, ok := firstOfType[*microflows.LoopedActivity](fb.objects)
	if !ok {
		t.Fatal("no loop")
	}
	if loopObj.GetPosition().Y <= split.GetPosition().Y {
		t.Errorf("loop at %v is not under the split at %v", loopObj.GetPosition(), split.GetPosition())
	}
}

func TestIfElseColumn_AppliesInsideALoopBody(t *testing.T) {
	// A loop body is built by a nested builder; it must draw in the flow's style.
	loop := &ast.LoopStmt{LoopVariable: "Item", ListVariable: "Items",
		Body: []ast.MicroflowStatement{ifElse(logStatements(1), logStatements(2))}}
	fb := buildWithLayout(t, []ast.MicroflowStatement{loop}, columnLayout())
	l, ok := firstOfType[*microflows.LoopedActivity](fb.objects)
	if !ok {
		t.Fatal("no loop")
	}
	body := l.ObjectCollection.Objects
	split, _ := firstOfType[*microflows.ExclusiveSplit](body)
	acts := activityPositions(body)
	if len(acts) != 3 || acts[1].X != split.GetPosition().X || acts[2].X != split.GetPosition().X || acts[2].Y <= acts[1].Y {
		t.Fatalf("ELSE body in the loop at %v, want a column under the split at %v", acts, split.GetPosition())
	}
	ls := sizeOf(l)
	for _, p := range acts {
		if p.Y+ActivityHeight/2 > ls.Height || p.X+ActivityWidth/2 > ls.Width {
			t.Errorf("activity at %v sticks out of the loop box %v", p, ls)
		}
	}
}
