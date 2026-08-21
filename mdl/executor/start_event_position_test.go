// SPDX-License-Identifier: Apache-2.0

// Where a microflow's StartEvent lands across a rewrite, and why it takes three
// sources to get right.
//
// The start has no statement of its own, so the builder derives its position:
// one spacing unit left of the first activity, on that activity's centre line.
// Two reports pull that derivation in opposite directions.
//
//   - #884: a Studio Pro flow whose start sat at 145;200 came back at 100;200 on
//     a describe→exec round-trip — 145 is not derivable from 260, and every
//     other coordinate in the flow survived. Carrying the stored value over
//     fixed it.
//   - #951: carrying it over UNCONDITIONALLY pinned the start of every rewritten
//     flow. Measured on a real project: activities moved to 360;340 by the very
//     script doing the rewrite, start left behind at 40;200.
//
// Neither is answerable without asking where the stored value came from, which
// authoredStartPosition does — a start at the derived spot is mxcli's own
// arithmetic handed back and carries no intent; a start anywhere else was placed
// by a person. On top of that, @start(x, y) states the position outright, which
// is what DESCRIBE emits for a start that arithmetic cannot reconstruct.
package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func startEventOf(objects []microflows.MicroflowObject) *microflows.StartEvent {
	for _, o := range objects {
		if se, ok := o.(*microflows.StartEvent); ok {
			return se
		}
	}
	return nil
}

// buildFlowWithStart builds a one-activity flow whose activity is annotated at
// 260;200, so the derived start is 100;200. start is the carried-over position
// (nil for a fresh CREATE); explicitStart is an @start(x, y) on the statement.
func buildFlowWithStart(t *testing.T, start *model.Point, explicitStart *ast.Position) *microflows.StartEvent {
	t.Helper()
	fb := &flowBuilder{
		posX: 100, posY: 200, baseY: 200, spacing: HorizontalSpacing,
		varTypes:      map[string]string{},
		declaredVars:  map[string]string{},
		measurer:      &layoutMeasurer{},
		startPosition: start,
	}
	stmt := &ast.LogStmt{
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"},
		Annotations: &ast.ActivityAnnotations{
			Position: &ast.Position{X: 260, Y: 200},
			Start:    explicitStart,
		},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{stmt}, nil)
	se := startEventOf(fb.objects)
	if se == nil {
		t.Fatal("no StartEvent in the built flow")
	}
	return se
}

// The stored position wins over the derived one.
func TestStartEvent_PreservesStoredPosition(t *testing.T) {
	se := buildFlowWithStart(t, &model.Point{X: 145, Y: 200}, nil)
	if se.Position.X != 145 || se.Position.Y != 200 {
		t.Errorf("StartEvent at %d;%d, want 145;200 — a hand-laid-out start must survive a rebuild",
			se.Position.X, se.Position.Y)
	}
}

// With nothing stored (a fresh CREATE) the derived placement is unchanged:
// one spacing unit left of the first annotated activity.
func TestStartEvent_DerivesWhenNothingStored(t *testing.T) {
	se := buildFlowWithStart(t, nil, nil)
	if want := 260 - HorizontalSpacing; se.Position.X != want {
		t.Errorf("StartEvent at %d, want %d (derived)", se.Position.X, want)
	}
}

// @start(x, y) beats a carried-over position: it states where the start goes,
// where the carry-over only infers it. Without this there is no way to move a
// start once one has been preserved. (#951)
func TestStartEvent_ExplicitAnnotationBeatsTheCarriedOverPosition(t *testing.T) {
	se := buildFlowWithStart(t, &model.Point{X: 145, Y: 200}, &ast.Position{X: 500, Y: 620})
	if se.Position.X != 500 || se.Position.Y != 620 {
		t.Errorf("StartEvent at %d;%d, want 500;620 — @start must win over the carry-over",
			se.Position.X, se.Position.Y)
	}
}

// --- Which stored positions are carried over at all (#951) ---

// oneActivityFlow is a stored flow: start → activity → end, with the start and
// the activity where the caller puts them.
func oneActivityFlow(start, activity model.Point) *microflows.MicroflowObjectCollection {
	se := &microflows.StartEvent{BaseMicroflowObject: mkObj("start")}
	se.Position = start
	act := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act")},
	}
	act.Position = activity
	end := &microflows.EndEvent{BaseMicroflowObject: mkObj("end")}
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{se, act, end},
		Flows:   []*microflows.SequenceFlow{mkFlow("start", "act"), mkFlow("act", "end")},
	}
}

func TestAuthoredStartPosition(t *testing.T) {
	cases := []struct {
		name     string
		start    model.Point
		activity model.Point
		want     *model.Point
	}{{
		// #884: Studio Pro put the start at 145 where the layout would put 100.
		name:     "hand-placed start is carried over",
		start:    model.Point{X: 145, Y: 200},
		activity: model.Point{X: 260, Y: 200},
		want:     &model.Point{X: 145, Y: 200},
	}, {
		// #951: mxcli's own arithmetic, handed back. Carrying it over is what
		// stranded the start when the next rewrite moved the activities.
		name:     "start at the derived spot is not carried over",
		start:    model.Point{X: 40, Y: 200},
		activity: model.Point{X: 200, Y: 200},
		want:     nil,
	}, {
		// Same X, different centre line — a start left on the old baseline
		// while the flow moved down is exactly the diagonal from #951.
		name:     "start off the activity centre line is carried over",
		start:    model.Point{X: 200, Y: 200},
		activity: model.Point{X: 360, Y: 340},
		want:     &model.Point{X: 200, Y: 200},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authoredStartPosition(oneActivityFlow(tc.start, tc.activity))
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("carried over %d;%d, want nothing carried over — the rebuild must "+
					"re-derive it from wherever the activities now are", got.X, got.Y)
			case tc.want != nil && got == nil:
				t.Fatalf("carried over nothing, want %d;%d — a position nobody can "+
					"reconstruct by arithmetic must survive the rebuild", tc.want.X, tc.want.Y)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("carried over %d;%d, want %d;%d", got.X, got.Y, tc.want.X, tc.want.Y)
			}
		})
	}
}

// A flow the derivation cannot be run against yields no carry-over rather than a
// wrong one: with no first object there is nothing to compare the start to, so
// its provenance is unknown and inventing an answer either pins a derived start
// or discards a hand-placed one.
func TestAuthoredStartPosition_UnderivableFlowsCarryNothing(t *testing.T) {
	se := &microflows.StartEvent{BaseMicroflowObject: mkObj("start")}
	se.Position = model.Point{X: 145, Y: 200}

	for _, tc := range []struct {
		name string
		oc   *microflows.MicroflowObjectCollection
	}{
		{"nil collection", nil},
		{"no start event", &microflows.MicroflowObjectCollection{}},
		{"start flows nowhere", &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{se},
		}},
		{"start flows to an object that is not in the collection", &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{se},
			Flows:   []*microflows.SequenceFlow{mkFlow("start", "missing")},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := authoredStartPosition(tc.oc); got != nil {
				t.Errorf("carried over %d;%d, want nothing", got.X, got.Y)
			}
		})
	}
}

// derivedStartPosition is buildFlowGraph's placement rule read backwards, so the
// two have to agree — a builder that changed its spacing or its centre line
// while the reader did not would classify every generated start as hand-placed
// and pin it, which is #951 again with a longer fuse.
//
// Asserted by running the builder and the reader over the same flow rather than
// by restating the arithmetic, which would pass against either one being wrong.
func TestDerivedStartPositionMatchesTheBuilder(t *testing.T) {
	built := buildFlowWithStart(t, nil, nil)

	act := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act")},
	}
	act.Position = model.Point{X: 260, Y: 200}
	derived, ok := derivedStartPosition(oneActivityFlow(built.Position, act.Position))
	if !ok {
		t.Fatal("derivedStartPosition found nothing to derive from")
	}
	if derived != built.Position {
		t.Errorf("the reader derives %d;%d where the builder places %d;%d — "+
			"the two placements have drifted apart",
			derived.X, derived.Y, built.Position.X, built.Position.Y)
	}
}

// --- What DESCRIBE emits (#951) ---

// Both describers, because they are near-duplicates: DESCRIBE MICROFLOW goes
// through formatMicroflowActivities and the ELK/source-map path through
// formatMicroflowActivitiesWithSourceMap. Testing one leaves the round-trip
// depending on which command the author happened to run — the first cut of this
// fix emitted from the source-map one only, and DESCRIBE dropped the line.
func TestDescribeEmitsStartAnnotationOnlyWhenItCarriesInformation(t *testing.T) {
	ctx := newTestExecutor().newExecContext(context.Background())

	describers := map[string]func(*microflows.Microflow) []string{
		"formatMicroflowActivities": func(mf *microflows.Microflow) []string {
			return formatMicroflowActivities(ctx, mf, nil, nil)
		},
		"formatMicroflowActivitiesWithSourceMap": func(mf *microflows.Microflow) []string {
			return formatMicroflowActivitiesWithSourceMap(ctx, mf, nil, nil, nil, 0)
		},
	}

	for name, describe := range describers {
		t.Run(name+"/hand-placed start is emitted", func(t *testing.T) {
			mf := &microflows.Microflow{
				ObjectCollection: oneActivityFlow(model.Point{X: 145, Y: 200}, model.Point{X: 260, Y: 200}),
			}
			out := strings.Join(describe(mf), "\n")
			if !strings.Contains(out, "@start(145, 200)") {
				t.Errorf("no @start line — a start at 145 cannot be reconstructed from a "+
					"description that does not mention it:\n%s", out)
			}
		})

		t.Run(name+"/derived start is not emitted", func(t *testing.T) {
			mf := &microflows.Microflow{
				ObjectCollection: oneActivityFlow(model.Point{X: 100, Y: 200}, model.Point{X: 260, Y: 200}),
			}
			out := strings.Join(describe(mf), "\n")
			if strings.Contains(out, "@start(") {
				t.Errorf("@start emitted for a start the layout would place there anyway — "+
					"every description would grow a line restating its own arithmetic, and "+
					"pin the start of every flow it round-trips:\n%s", out)
			}
		})
	}
}
