// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1832: `guard: true-right` sends a guard's TRUE branch — the early
// return — straight on to the right, and the happy path down from the split's
// bottom onto a new line starting in the split's column.

func guardRightLayout() *flowLayout { return &flowLayout{Guard: GuardTrueRight} }

func TestGuardTrueRight_ReturnOnTheLineHappyPathBelow(t *testing.T) {
	stmts := append(logStatements(1), guardStmt())
	stmts = append(stmts, logStatements(2)...)
	fb := buildWithLayout(t, stmts, guardRightLayout())

	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	sp := split.GetPosition()
	acts := activityPositions(fb.objects)
	if len(acts) != 4 {
		t.Fatalf("want 4 activities, got %d", len(acts))
	}
	before, branch, after := acts[0], acts[1], acts[2:]

	if before.Y != sp.Y || branch.Y != sp.Y || branch.X <= sp.X {
		t.Errorf("guard branch at %v, want on the centre line y=%d right of the split at x=%d", branch, sp.Y, sp.X)
	}
	if after[0].X != sp.X {
		t.Errorf("first activity after the guard at x=%d, want the split's column x=%d", after[0].X, sp.X)
	}
	if after[1].Y != after[0].Y || after[1].X <= after[0].X {
		t.Errorf("happy path after the guard %v is not a horizontal line", after)
	}
	// The new line clears everything the guard drew, end event included.
	var lowest int
	for _, o := range fb.objects {
		if o.GetPosition().Y == sp.Y {
			lowest = max(lowest, o.GetPosition().Y+sizeOf(o).Height/2)
		}
	}
	if top := after[0].Y - ActivityHeight/2; top-lowest != RowGap {
		t.Errorf("new line's top y=%d is %d px under the guard's line (bottom %d), want RowGap=%d", top, top-lowest, lowest, RowGap)
	}

	ids := activityIDs(fb.objects)
	if f := flowBetween(fb.flows, split.GetID(), ids[1]); f == nil || f.OriginConnectionIndex != AnchorRight || f.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("TRUE flow = %+v, want split right -> branch left", f)
	}
	f := flowBetween(fb.flows, split.GetID(), ids[2])
	if f == nil || f.OriginConnectionIndex != AnchorBottom || f.DestinationConnectionIndex != AnchorTop {
		t.Errorf("FALSE flow = %+v, want split bottom -> top of the next activity", f)
	}
	if ec, ok := f.CaseValue.(*microflows.ExpressionCase); f != nil && (!ok || ec.Expression != "false") {
		t.Errorf("FALSE flow case = %+v, want false", f.CaseValue)
	}
	assertNoOverlap(t, fb.objects)
}

func TestGuardTrueRight_DefaultLayoutIsUnchanged(t *testing.T) {
	// Control: without the option the branch is in the lane below and the happy
	// path stays on the centre line.
	stmts := append([]ast.MicroflowStatement{guardStmt()}, logStatements(1)...)
	fb := buildWithLayout(t, stmts, &flowLayout{})
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	acts := activityPositions(fb.objects)
	if acts[0].Y <= split.GetPosition().Y || acts[1].Y != split.GetPosition().Y {
		t.Fatalf("default guard: branch at %v, happy path at %v, split at %v", acts[0], acts[1], split.GetPosition())
	}
}

func TestGuardTrueRight_InsideAnIfBranchKeepsTheElseLaneClear(t *testing.T) {
	// A guard in a THEN body moves the rest of that body down; the ELSE lane under
	// it must be placed below all of it, which takes the measurer knowing.
	thenBody := append([]ast.MicroflowStatement{guardStmt()}, logStatements(2)...)
	stmts := []ast.MicroflowStatement{ifElse(thenBody, logStatements(2))}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, guardRightLayout())
	assertNoOverlap(t, fb.objects)

	// The guard's new line belongs to the THEN body: after the IF the flow is
	// back on the centre line, its end event too.
	split, _ := firstOfType[*microflows.ExclusiveSplit](fb.objects)
	acts := activityPositions(fb.objects)
	if last := acts[len(acts)-1]; last.Y != split.GetPosition().Y {
		t.Errorf("activity after the IF at %v, want the centre line y=%d", last, split.GetPosition().Y)
	}
	end := fb.objects[len(fb.objects)-1]
	if _, ok := end.(*microflows.EndEvent); !ok || end.GetPosition().Y != split.GetPosition().Y {
		t.Errorf("flow's end event %T at %v, want an end event on the centre line y=%d", end, end.GetPosition(), split.GetPosition().Y)
	}
}

func TestGuardTrueRight_SeveralGuardsStepDown(t *testing.T) {
	stmts := []ast.MicroflowStatement{guardStmt(), guardStmt(), guardStmt()}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, guardRightLayout())
	var ys []int
	for _, o := range fb.objects {
		if _, ok := o.(*microflows.ExclusiveSplit); ok {
			ys = append(ys, o.GetPosition().Y)
		}
	}
	if len(ys) != 3 || !(ys[0] < ys[1] && ys[1] < ys[2]) {
		t.Fatalf("guard splits at y=%v, want each on its own line below the last", ys)
	}
	assertNoOverlap(t, fb.objects)
}

func TestGuardTrueRight_InsideALoopFitsTheBox(t *testing.T) {
	body := append([]ast.MicroflowStatement{guardStmt()}, logStatements(2)...)
	loop := &ast.LoopStmt{LoopVariable: "Item", ListVariable: "Items", Body: body}
	fb := buildWithLayout(t, []ast.MicroflowStatement{loop}, guardRightLayout())
	l, ok := firstOfType[*microflows.LoopedActivity](fb.objects)
	if !ok {
		t.Fatal("no loop")
	}
	ls := sizeOf(l)
	for _, o := range l.ObjectCollection.Objects {
		p, s := o.GetPosition(), sizeOf(o)
		if p.Y+s.Height/2 > ls.Height || p.X+s.Width/2 > ls.Width || p.Y-s.Height/2 < 0 {
			t.Errorf("%T at %v sticks out of the loop box %v", o, p, ls)
		}
	}
	assertNoOverlap(t, l.ObjectCollection.Objects)
}

func TestGuardTrueRight_CombinesWithElseColumn(t *testing.T) {
	elseBody := append([]ast.MicroflowStatement{guardStmt()}, logStatements(2)...)
	stmts := []ast.MicroflowStatement{ifElse(logStatements(1), elseBody)}
	stmts = append(stmts, logStatements(1)...)
	fb := buildWithLayout(t, stmts, &flowLayout{IfElse: IfElseColumn, Guard: GuardTrueRight})
	assertNoOverlap(t, fb.objects)
}
