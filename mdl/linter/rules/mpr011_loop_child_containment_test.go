// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func loopChild(caption string, x, y int) *microflows.ActionActivity {
	a := &microflows.ActionActivity{}
	a.Position = model.Point{X: x, Y: y}
	a.Size = model.Size{Width: 120, Height: 60}
	a.Caption = caption
	return a
}

func loopWith(w, h int, children ...microflows.MicroflowObject) *microflows.LoopedActivity {
	loop := &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: children},
	}
	loop.Position = model.Point{X: 500, Y: 200}
	loop.Size = model.Size{Width: w, Height: h}
	loop.Caption = "loop"
	return loop
}

func TestLoopChildContainment_FlagsAChildOutsideItsBox(t *testing.T) {
	// The #884 case: the box is 480 wide, the child sits at x=2000.
	loop := loopWith(480, 160, loopChild("two", 2000, 60))
	got := escapedLoopChildren([]microflows.MicroflowObject{loop})
	if len(got) != 1 {
		t.Fatalf("expected 1 escapee, got %d: %+v", len(got), got)
	}
	if got[0].Child != "two" {
		t.Errorf("child = %q, want %q", got[0].Child, "two")
	}
}

func TestLoopChildContainment_AcceptsAContainedChild(t *testing.T) {
	// x=310 with a 120-wide box spans [250,370], inside a 480-wide container.
	loop := loopWith(480, 160, loopChild("one", 310, 80))
	if got := escapedLoopChildren([]microflows.MicroflowObject{loop}); len(got) != 0 {
		t.Errorf("a contained child must not be flagged, got %+v", got)
	}
}

// A child at the origin is unpositioned rather than escaped — MPR008 skips
// those for the same reason, and flagging them would bury the real cases.
func TestLoopChildContainment_IgnoresUnpositionedChildren(t *testing.T) {
	loop := loopWith(480, 160, loopChild("unpositioned", 0, 0))
	if got := escapedLoopChildren([]microflows.MicroflowObject{loop}); len(got) != 0 {
		t.Errorf("an unpositioned child must not be flagged, got %+v", got)
	}
}

// Loops inside loops: the inner container is checked against its own box, in
// its own coordinate space — the same distinction MPR008 needed (#884).
func TestLoopChildContainment_ChecksNestedLoops(t *testing.T) {
	inner := loopWith(200, 100, loopChild("deep", 900, 50))
	inner.Caption = "inner"
	inner.Position = model.Point{X: 150, Y: 80}
	outer := loopWith(600, 200, inner)

	got := escapedLoopChildren([]microflows.MicroflowObject{outer})
	if len(got) != 1 {
		t.Fatalf("expected the nested escapee, got %d: %+v", len(got), got)
	}
	if got[0].Child != "deep" || got[0].Loop != "inner" {
		t.Errorf("got child=%q loop=%q, want child=deep loop=inner", got[0].Child, got[0].Loop)
	}
}

func TestLoopChildContainmentRule_Metadata(t *testing.T) {
	r := NewLoopChildContainmentRule()
	if r.ID() != "MPR011" {
		t.Errorf("ID = %q, want MPR011", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
}

func TestLoopChildContainmentRule_NilReader(t *testing.T) {
	r := NewLoopChildContainmentRule()
	if v := r.Check(linter.NewLintContextFromDB(nil)); v != nil {
		t.Errorf("expected nil with nil reader, got %v", v)
	}
}
