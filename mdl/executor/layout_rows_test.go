// SPDX-License-Identifier: Apache-2.0

// A long main line used to run off the canvas: every top-level statement advanced
// posX and nothing ever moved down. Measured on a generated app of 41 microflows
// with no @position anywhere, the widest flow was 6930x160 px — 43:1, four screens
// of horizontal scrolling. Nothing overlapped; it simply could not be read.
package executor

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestShortFlowIsNotWrapped(t *testing.T) {
	// Eight activities span 200..1320, inside MaxRowWidth: the coordinates every
	// existing layout test pins must not move.
	positions := activityPositions(buildRows(t, logStatements(8)))
	if len(positions) != 8 {
		t.Fatalf("expected 8 activities, got %d", len(positions))
	}
	for i, p := range positions {
		wantX := 360 + i*HorizontalSpacing
		if p.X != wantX || p.Y != 200 {
			t.Fatalf("activity %d at (%d,%d), want (%d,200) — a flow that fits must lay out exactly as before",
				i, p.X, p.Y, wantX)
		}
	}
}

func TestLongFlowWrapsOntoRowsThatDoNotTouch(t *testing.T) {
	positions := activityPositions(buildRows(t, logStatements(30)))
	if len(positions) != 30 {
		t.Fatalf("expected 30 activities, got %d", len(positions))
	}

	var maxX, rows int
	lastY := positions[0].Y
	for _, p := range positions {
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y != lastY {
			rows++
			lastY = p.Y
		}
	}
	if rows == 0 {
		t.Fatalf("30 activities stayed on one row %d px wide; the flow should have wrapped", maxX)
	}
	// The row starts at the first activity's column, one spacing past the start event.
	rowStart := positions[0].X
	if maxX-rowStart > MaxRowWidth {
		t.Fatalf("row runs from x=%d to x=%d, %d px wide, past MaxRowWidth=%d",
			rowStart, maxX, maxX-rowStart, MaxRowWidth)
	}

	// No two activities may share space, and rows keep RowGap between them.
	for i := 0; i < len(positions); i++ {
		for j := i + 1; j < len(positions); j++ {
			dx := abs(positions[i].X - positions[j].X)
			dy := abs(positions[i].Y - positions[j].Y)
			if dx < ActivityWidth && dy < ActivityHeight {
				t.Fatalf("activities %d and %d overlap: (%d,%d) and (%d,%d)",
					i, j, positions[i].X, positions[i].Y, positions[j].X, positions[j].Y)
			}
		}
	}
}

func TestWrapEdgeLeavesTheBottomOfTheRow(t *testing.T) {
	// The edge from the last element of a row to the first of the next is the one
	// flow that travels backwards. Anchored right-to-left it is drawn straight
	// across the row it just left; arriving on the left it still comes in from the
	// right, over the first activities of the new row. Bottom to top keeps it in the
	// band between the rows.
	objects := buildRows(t, logStatements(30))
	positions := activityPositions(objects)
	byID := map[model.ID]model.Point{}
	for _, o := range objects {
		byID[o.GetID()] = o.GetPosition()
	}
	_ = positions

	fb := &flowBuilder{
		posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing, allowWrap: true,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}},
	}
	fb.buildFlowGraph(logStatements(30), nil)

	var wrapEdges int
	for _, flow := range fb.flows {
		from, okFrom := byIDPosition(fb.objects, flow.OriginID)
		to, okTo := byIDPosition(fb.objects, flow.DestinationID)
		if !okFrom || !okTo || to.Y <= from.Y || to.X >= from.X {
			continue // not a backwards, downwards edge
		}
		wrapEdges++
		if flow.OriginConnectionIndex != AnchorBottom || flow.DestinationConnectionIndex != AnchorTop {
			t.Fatalf("wrap edge (%d,%d)->(%d,%d) anchored %d->%d, want bottom(%d)->top(%d)",
				from.X, from.Y, to.X, to.Y,
				flow.OriginConnectionIndex, flow.DestinationConnectionIndex, AnchorBottom, AnchorTop)
		}
	}
	if wrapEdges == 0 {
		t.Fatal("no row-crossing edge found; the flow did not wrap")
	}
}

// loopOverBranch is a loop whose body is an IF with both branches: a box the measurer
// sizes smaller than fitContainerSize makes it once the body is laid out.
func loopOverBranch() *ast.LoopStmt {
	return &ast.LoopStmt{LoopVariable: "Item", ListVariable: "List", Body: []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			ThenBody:  logStatements(2),
			ElseBody:  logStatements(2),
			HasElse:   true,
		},
	}}
}

// rowBoxes returns the top and bottom edge of every loop box, grouped by row (boxes
// sharing a centre line), rows in top-to-bottom order.
func rowBoxes(objects []microflows.MicroflowObject) (tops, bottoms []int) {
	byRow := map[int][2]int{}
	var rows []int
	for _, o := range objects {
		if _, ok := o.(*microflows.LoopedActivity); !ok {
			continue
		}
		h := o.(interface{ GetSize() model.Size }).GetSize().Height
		y := o.GetPosition().Y
		edges, seen := byRow[y]
		if !seen {
			rows = append(rows, y)
			edges = [2]int{y - h/2, y + h/2}
		}
		byRow[y] = [2]int{min(edges[0], y-h/2), max(edges[1], y+h/2)}
	}
	sort.Ints(rows)
	for _, y := range rows {
		tops = append(tops, byRow[y][0])
		bottoms = append(bottoms, byRow[y][1])
	}
	return tops, bottoms
}

func TestTallElementStartingARowClearsTheRowAbove(t *testing.T) {
	// A loop box hangs half its height above its row's centre line, so a row of loops
	// begun at an activity's depth reaches into the row above — two loops on top of
	// each other — unless the finished rows are moved apart.
	stmts := make([]ast.MicroflowStatement, 0, 12)
	for i := 0; i < 12; i++ {
		stmts = append(stmts, loopOverBranch())
	}
	fb := &flowBuilder{posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing, allowWrap: true,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}}}
	fb.buildFlowGraph(stmts, nil)
	tops, bottoms := rowBoxes(fb.objects)
	if len(tops) < 2 {
		t.Fatalf("expected the loops to wrap onto several rows, got %d", len(tops))
	}
	for r := 1; r < len(tops); r++ {
		if tops[r] < bottoms[r-1]+RowGap {
			t.Fatalf("row %d starts at y=%d, %d px under the row above (want at least %d)",
				r, tops[r], tops[r]-bottoms[r-1], RowGap)
		}
	}
}

func TestRowsKeepTheirGapAfterContainersAreSized(t *testing.T) {
	// A loop box is sized from its body after the body is laid out, and the
	// measurer's estimate for the same loop is smaller, so a row placed on the
	// estimate alone ends up closer to the row above than RowGap. separateRows
	// measures what was built. Calling it on a flow whose rows were deliberately
	// pulled together shows it is the pass that restores the gap.
	stmts := make([]ast.MicroflowStatement, 0, 12)
	for i := 0; i < 12; i++ {
		stmts = append(stmts, loopOverBranch())
	}
	fb := &flowBuilder{posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing, allowWrap: true,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}}}
	fb.buildFlowGraph(stmts, nil)
	if len(fb.row.starts) < 2 {
		t.Fatalf("expected several rows, got %d", len(fb.row.starts))
	}
	// Pull the second row up into the first, as an under-estimated reservation would.
	for _, o := range fb.objects[fb.row.starts[1]:] {
		p := o.GetPosition()
		o.SetPosition(model.Point{X: p.X, Y: p.Y - 150})
	}
	if tops, bottoms := rowBoxes(fb.objects); tops[1] >= bottoms[0]+RowGap {
		t.Fatal("test setup: the rows still clear each other, so there is nothing to repair")
	}
	fb.separateRows()
	tops, bottoms := rowBoxes(fb.objects)
	for r := 1; r < len(tops); r++ {
		if gap := tops[r] - bottoms[r-1]; gap < RowGap {
			t.Fatalf("row %d is %d px under the row above after separateRows (want at least %d)", r, gap, RowGap)
		}
	}
}

func TestFlowSlightlyOverTheLimitFinishesOnItsRow(t *testing.T) {
	// 20 activities run 80px past MaxRowWidth. Wrapping there left two activities and
	// the end event on a row of their own; the row is allowed to overhang instead.
	positions := activityPositions(buildRows(t, logStatements(20)))
	for i, p := range positions {
		if p.Y != positions[0].Y {
			t.Fatalf("activity %d of 20 wrapped to y=%d; the tail is short enough to stay on the row", i, p.Y)
		}
	}
	if span := positions[len(positions)-1].X - positions[0].X + ActivityWidth; span <= MaxRowWidth {
		t.Fatalf("test flow spans %d, inside MaxRowWidth=%d: it does not exercise the overhang", span, MaxRowWidth)
	}
}
