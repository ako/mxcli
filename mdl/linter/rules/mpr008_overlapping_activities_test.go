// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// NOTE: The full Check() logic requires ctx.Reader().GetMicroflow() to read microflow
// positions from a real MPR file. The overlap detection algorithm (collect positions →
// pairwise distance check) is inline in Check() and cannot be unit-tested without
// building a mock mpr.Reader. This rule currently lacks end-to-end coverage;
// behavioral testing requires a real .mpr project with overlapping activities.

func TestOverlappingActivitiesRule_NilReader(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestOverlappingActivitiesRule_Metadata(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	if r.ID() != "MPR008" {
		t.Errorf("ID = %q, want MPR008", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
	if r.Name() != "OverlappingActivities" {
		t.Errorf("Name = %q, want OverlappingActivities", r.Name())
	}
}

// The Check() method walks microflow positions via ctx.Reader().GetMicroflow() and detects
// overlapping activities using pairwise distance checks against internal heuristic constants
// (activityBoxWidth, activityBoxHeight). Since the collect function is defined inline in
// Check(), behavioral testing requires a real *mpr.Reader with positioned activities.

// upstream #884. MPR008 flattened a loop's children into the same list as the
// microflow's own canvas and compared every pair. A LoopedActivity's children are
// positioned RELATIVE to the container, so that compares two different coordinate
// spaces and reports overlaps that cannot happen on screen.
//
// Verified against a real project before and after: an outer activity at
// (150,150) and a loop child at (141,130) were reported as overlapping by the
// pre-fix binary and are not by the fixed one, while a genuine same-canvas pair
// at (300,200)/(310,210) is still reported by both.
func TestOverlapPlanesSeparatesContainerCanvases(t *testing.T) {
	inner := &microflows.ActionActivity{}
	inner.Position = model.Point{X: 141, Y: 130}
	inner.Caption = "inner"

	loop := &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}
	loop.Position = model.Point{X: 520, Y: 230}
	loop.Caption = "loop"

	outer := &microflows.ActionActivity{}
	outer.Position = model.Point{X: 150, Y: 150}
	outer.Caption = "outer"

	planes := overlapPlanes([]microflows.MicroflowObject{outer, loop})

	if len(planes) != 2 {
		t.Fatalf("got %d planes, want 2 (the microflow canvas and the loop's own)", len(planes))
	}

	// The container sits on the parent's plane; only its children move.
	captions := map[string]int{}
	for i, plane := range planes {
		for _, a := range plane {
			captions[a.caption] = i
		}
	}
	if captions["outer"] != captions["loop"] {
		t.Errorf("the loop container must share the parent canvas with 'outer': outer=%d loop=%d",
			captions["outer"], captions["loop"])
	}
	if captions["inner"] == captions["outer"] {
		t.Error("the loop's CHILD must not share a plane with the microflow's own canvas — " +
			"its coordinates are relative to the container, so comparing them is meaningless")
	}
}

// A container with no children must not produce a phantom plane holding nothing,
// and a flat microflow must produce exactly one.
func TestOverlapPlanesFlatMicroflow(t *testing.T) {
	a := &microflows.ActionActivity{}
	a.Position = model.Point{X: 300, Y: 200}
	b := &microflows.ActionActivity{}
	b.Position = model.Point{X: 310, Y: 210}

	planes := overlapPlanes([]microflows.MicroflowObject{a, b})
	if len(planes) != 1 {
		t.Fatalf("got %d planes, want 1", len(planes))
	}
	if len(planes[0]) != 2 {
		t.Errorf("got %d activities on the canvas, want 2 — both must still be compared "+
			"against each other, or the fix trades a false positive for a false negative", len(planes[0]))
	}
}
