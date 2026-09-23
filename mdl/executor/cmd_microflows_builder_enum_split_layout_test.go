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

func buildEnumSplit(t *testing.T, cases int) (*flowBuilder, model.ID) {
	t.Helper()
	fb := &flowBuilder{posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}}}
	fb.buildFlowGraph([]ast.MicroflowStatement{enumSplitWithBranchCount(cases)}, nil)
	for _, o := range fb.objects {
		if _, ok := o.(*microflows.ExclusiveSplit); ok {
			return fb, o.GetID()
		}
	}
	t.Fatal("no split built")
	return nil, ""
}

func TestEnumCaseLinesLeaveTheSplitInThreeGroups(t *testing.T) {
	// Seven cases: the two upper branches leave the split's top corner, the three in
	// the middle its right, the two lower its bottom, and every one arrives at the
	// left of its activity. The order table this replaces sent the fourth case out of
	// the split's LEFT corner and the fifth onwards onto the top of their activities,
	// so the lines crossed each other and the activities in between.
	fb, splitID := buildEnumSplit(t, 7)
	want := map[string]int{
		"Value1": AnchorTop, "Value2": AnchorTop,
		"Value3": AnchorRight, "Value4": AnchorRight, "Value5": AnchorRight,
		"Value6": AnchorBottom, "Value7": AnchorBottom,
	}
	seen := 0
	for _, flow := range fb.flows {
		if flow.OriginID != splitID {
			continue
		}
		value, ok := enumCaseValue(flow)
		if !ok {
			continue
		}
		seen++
		if flow.OriginConnectionIndex != want[value] || flow.DestinationConnectionIndex != AnchorLeft {
			t.Errorf("%s leaves side %d and arrives side %d, want %d and left(%d)",
				value, flow.OriginConnectionIndex, flow.DestinationConnectionIndex, want[value], AnchorLeft)
		}
		// A line from the top corner only ever goes up, one from the bottom only down.
		split, _ := byIDPosition(fb.objects, splitID)
		dest, _ := byIDPosition(fb.objects, flow.DestinationID)
		if want[value] == AnchorTop && dest.Y >= split.Y || want[value] == AnchorBottom && dest.Y <= split.Y {
			t.Errorf("%s leaves side %d towards y=%d with the split at y=%d", value, want[value], dest.Y, split.Y)
		}
	}
	if seen != 7 {
		t.Fatalf("found %d case flows, want 7", seen)
	}
}

func TestEnumCaseGroupSizes(t *testing.T) {
	for n, want := range map[int][3]int{4: {1, 2, 1}, 5: {2, 1, 2}, 6: {2, 2, 2}, 7: {2, 3, 2}, 8: {3, 2, 3}, 9: {3, 3, 3}} {
		ys := make([]int, n)
		for i := range ys {
			ys[i] = (i*2 - (n - 1)) * 50 // stacked symmetrically around 0
		}
		var got [3]int
		for _, side := range enumSplitOriginAnchors(ys, 0) {
			got[side]++ // AnchorTop=0, AnchorRight=1, AnchorBottom=2
		}
		if got != want {
			t.Errorf("%d cases grouped %v, want %v", n, got, want)
		}
	}
	if enumSplitOriginAnchors([]int{-100, 0, 100}, 0) != nil {
		t.Error("three cases must keep the pair table: it already draws top, right, bottom")
	}
}

func TestDescribeReadsCaseOrderFromGroupedLines(t *testing.T) {
	// The pair table existed to store the case order, which the stored flow order
	// does not survive. Grouped lines store it as side, then depth on the canvas.
	for _, n := range []int{4, 7, 12} {
		fb, splitID := buildEnumSplit(t, n)
		objects := map[model.ID]microflows.MicroflowObject{}
		for _, o := range fb.objects {
			objects[o.GetID()] = o
		}
		var flows []*microflows.SequenceFlow
		for _, flow := range fb.flows {
			if flow.OriginID == splitID {
				flows = append(flows, flow)
			}
		}
		rand.New(rand.NewSource(int64(n))).Shuffle(len(flows), func(i, j int) { flows[i], flows[j] = flows[j], flows[i] })
		for i, flow := range orderedEnumSplitFlows(flows, objects) {
			if value, _ := enumCaseValue(flow); value != fmt.Sprintf("Value%d", i+1) {
				t.Fatalf("%d cases: position %d holds %s", n, i, value)
			}
		}
	}
}

func TestDescribeStillReadsThePairTable(t *testing.T) {
	// A model written before lines were grouped stores the order as one pair per case,
	// and nothing else: here every branch has the same Y, so only the pair can order
	// them.
	var flows []*microflows.SequenceFlow
	for i := 0; i < 9; i++ {
		flow := newHorizontalFlowWithEnumCase("split", model.ID(fmt.Sprintf("a%d", i)), fmt.Sprintf("Value%d", i+1))
		applySplitCaseOrder(flow, i)
		flows = append(flows, flow)
	}
	rand.New(rand.NewSource(9)).Shuffle(len(flows), func(i, j int) { flows[i], flows[j] = flows[j], flows[i] })
	for i, flow := range orderedEnumSplitFlows(flows, nil) {
		if value, _ := enumCaseValue(flow); value != fmt.Sprintf("Value%d", i+1) {
			t.Fatalf("position %d holds %s", i, value)
		}
	}
}

func TestHandPlacedCaseBranchesAreReadInCanvasOrder(t *testing.T) {
	// Deliberate, and a change from the pair table: with four or more cases the order
	// inside a group is the branch's place on the canvas, so branches a user has moved
	// with @position come back from DESCRIBE top to bottom, not in the order they were
	// typed. The model is unaffected — CASE branches have no order at run time — and a
	// second describe -> exec is a fixed point. Here the four branches are placed
	// bottom-up; none of them is on the side of the split its third was planned for, so
	// all four leave from the right and are read purely by Y.
	split := enumSplitWithBranchCount(4)
	for i := range split.Cases {
		log := split.Cases[i].Body[0].(*ast.LogStmt)
		log.Annotations = &ast.ActivityAnnotations{Position: &ast.Position{X: 700, Y: 500 - i*200}}
	}
	out := describeBuiltEnumSplitBody(t, []ast.MicroflowStatement{split})
	assertOrder(t, out, "when Value4", "when Value3", "when Value2", "when Value1")
}

func TestCaseLineNeverLeavesTheSplitAwayFromItsBranch(t *testing.T) {
	// The first case is counted into the upper third, but @position put its branch
	// under the split. Leaving from the top corner it would loop over the split to get
	// down there; it leaves from the right instead.
	split := enumSplitWithBranchCount(7)
	log := split.Cases[0].Body[0].(*ast.LogStmt)
	log.Annotations = &ast.ActivityAnnotations{Position: &ast.Position{X: 700, Y: 900}}

	fb := &flowBuilder{posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{varTypes: map[string]string{}}}
	fb.buildFlowGraph([]ast.MicroflowStatement{split}, nil)
	for _, flow := range fb.flows {
		if value, ok := enumCaseValue(flow); ok && value == "Value1" {
			if flow.OriginConnectionIndex != AnchorRight {
				t.Fatalf("Value1's branch is below the split but its line leaves side %d, want right(%d)",
					flow.OriginConnectionIndex, AnchorRight)
			}
			return
		}
	}
	t.Fatal("no flow for Value1")
}
