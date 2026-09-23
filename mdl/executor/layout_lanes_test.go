// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func guardStmt() *ast.IfStmt {
	return &ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "refused"}},
			&ast.ReturnStmt{},
		},
	}
}

func splitsOf(objects []microflows.MicroflowObject) []model.Point {
	var out []model.Point
	for _, o := range objects {
		if _, ok := o.(*microflows.ExclusiveSplit); ok {
			out = append(out, o.GetPosition())
		}
	}
	return out
}

func TestMainLineDoesNotWaitForAGuardsBranch(t *testing.T) {
	// The guard's branch is in the lane below and ends there. The activity after the
	// guard used to be placed past the branch's far end — 370px from the split, with
	// bare line over the branch; it belongs one pitch after the split.
	stmts := []ast.MicroflowStatement{guardStmt()}
	stmts = append(stmts, logStatements(1)...)
	objects := buildRows(t, stmts)

	split := splitsOf(objects)[0]
	var next, inBranch model.Point
	for _, p := range activityPositions(objects) {
		if p.Y == split.Y {
			next = p
		} else {
			inBranch = p
		}
	}
	if next.X != inBranch.X {
		t.Fatalf("activity after the guard at x=%d, branch starts at x=%d: they share a column", next.X, inBranch.X)
	}
	if got, want := next.X-split.X, SplitWidth+HorizontalSpacing/2; got != want {
		t.Fatalf("activity after the guard is %d px from the split, want %d", got, want)
	}
	if inBranch.Y-ActivityHeight/2 < next.Y+ActivityHeight/2 {
		t.Fatalf("branch activity at y=%d touches the main line activity at y=%d", inBranch.Y, next.Y)
	}
}

func TestSecondGuardWaitsForTheLaneToClear(t *testing.T) {
	// Two guards in a row both drop a branch into the same lane. The second split may
	// not start until the first branch — end event included — is out of the way.
	objects := buildRows(t, []ast.MicroflowStatement{guardStmt(), guardStmt()})
	splits := splitsOf(objects)
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	firstBranchRight := 0
	for _, o := range objects {
		p := o.GetPosition()
		if p.Y == splits[0].Y || p.X > splits[1].X {
			continue
		}
		w := ActivityWidth
		if ws, ok := o.(interface{ GetSize() model.Size }); ok && ws.GetSize().Width > 0 {
			w = ws.GetSize().Width
		}
		firstBranchRight = max(firstBranchRight, p.X+w/2)
	}
	if firstBranchRight == 0 {
		t.Fatal("found nothing in the first guard's branch")
	}
	if left := splits[1].X - SplitWidth/2; left < firstBranchRight {
		t.Fatalf("second split starts at x=%d, inside the first guard's branch (ends x=%d)", left, firstBranchRight)
	}
	// And the two branches themselves may not share space.
	var branch []model.Point
	for _, p := range activityPositions(objects) {
		if p.Y != splits[0].Y {
			branch = append(branch, p)
		}
	}
	if len(branch) != 2 || abs(branch[0].X-branch[1].X) < ActivityWidth {
		t.Fatalf("branch activities overlap: %v", branch)
	}
}

func TestMergeSitsOneGapAfterItsBranch(t *testing.T) {
	// 120px used to separate a branch's last activity from the merge, against 40px
	// between any two activities.
	objects := buildRows(t, []ast.MicroflowStatement{&ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody:  logStatements(2),
		ElseBody:  logStatements(1),
		HasElse:   true,
	}})
	var mergeLeft, lastRight int
	for _, o := range objects {
		switch o.(type) {
		case *microflows.ExclusiveMerge:
			mergeLeft = o.GetPosition().X - MergeSize/2
		case *microflows.ActionActivity:
			lastRight = max(lastRight, o.GetPosition().X+ActivityWidth/2)
		}
	}
	if gap := mergeLeft - lastRight; gap != HorizontalSpacing-ActivityWidth {
		t.Fatalf("merge is %d px after its branch, want %d", gap, HorizontalSpacing-ActivityWidth)
	}
}

func TestReturningBranchIsMeasuredWithItsEndEvent(t *testing.T) {
	// The merge is placed by the widest branch. A branch that ends in RETURN draws an
	// end event one pitch past its last activity; measured without it, a tightened
	// merge lands on that event.
	split := enumSplitWithBranchCount(3)
	split.Cases[1].Body = append(split.Cases[1].Body, &ast.ReturnStmt{}) // the branch on the centre line
	objects := buildRows(t, []ast.MicroflowStatement{split})

	var merge, end microflows.MicroflowObject
	for _, o := range objects {
		switch o.(type) {
		case *microflows.ExclusiveMerge:
			merge = o
		case *microflows.EndEvent:
			if end == nil {
				end = o
			}
		}
	}
	if merge == nil || end == nil {
		t.Fatal("expected a merge and a branch end event")
	}
	if end.GetPosition().Y == merge.GetPosition().Y &&
		merge.GetPosition().X-MergeSize/2 < end.GetPosition().X+EventSize/2 {
		t.Fatalf("merge at x=%d sits on the branch's end event at x=%d", merge.GetPosition().X, end.GetPosition().X)
	}
}
