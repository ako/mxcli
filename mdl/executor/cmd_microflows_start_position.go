// SPDX-License-Identifier: Apache-2.0

// Package executor - StartEvent placement across a microflow rewrite.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// derivedStartPosition returns where mxcli's own layout puts the StartEvent of a
// stored flow: one spacing unit left of the object the start flows into, on that
// object's centre line. This is buildFlowGraph's rule read backwards, and the two
// are pinned together by TestDerivedStartPositionMatchesTheBuilder.
//
// Reports false when the flow has no start, or the start leads nowhere — with no
// first object there is nothing to derive from, and nothing can be concluded
// about where the stored coordinates came from.
func derivedStartPosition(oc *microflows.MicroflowObjectCollection) (model.Point, bool) {
	if oc == nil {
		return model.Point{}, false
	}
	var startID model.ID
	byID := make(map[model.ID]microflows.MicroflowObject, len(oc.Objects))
	for _, o := range oc.Objects {
		byID[o.GetID()] = o
		if _, ok := o.(*microflows.StartEvent); ok {
			startID = o.GetID()
		}
	}
	if startID == "" {
		return model.Point{}, false
	}
	for _, f := range oc.Flows {
		if f.OriginID != startID {
			continue
		}
		first, ok := byID[f.DestinationID]
		if !ok {
			continue
		}
		p := first.GetPosition()
		return model.Point{X: p.X - HorizontalSpacing, Y: p.Y}, true
	}
	return model.Point{}, false
}

// authoredStartPosition returns the StartEvent position of a stored flow when
// those coordinates say something mxcli's own layout would not have produced —
// that is, when a person put the start there.
//
// The two reports this arbitrates are the same bug seen from opposite sides, and
// neither is answerable without asking where the stored value came from:
//
//   - #884: a Studio Pro flow whose start sat at 145;200 came back at 100;200
//     on a describe→exec round-trip. 145 is not derivable from anything in the
//     description, so a rebuild has to be told; carrying the stored value over
//     is how it is told.
//   - #951: carrying it over UNCONDITIONALLY then pinned the start of every
//     rewritten flow. Move the activities and the start stays behind — measured
//     at 40;200 while its own first activity had moved to 360;340, joined by a
//     long diagonal line across an otherwise empty canvas.
//
// A start at the derived spot is mxcli's own arithmetic handed back, so it
// carries no intent and is re-derived. A start anywhere else was placed by
// someone and is kept. A person who placed one exactly where the layout would
// have is indistinguishable from the layout — and re-deriving gives the same
// point for an unchanged flow, so the ambiguity costs nothing.
//
// Returns nil when there is no start, when nothing can be derived, or when the
// stored start is at the derived spot.
func authoredStartPosition(oc *microflows.MicroflowObjectCollection) *model.Point {
	derived, ok := derivedStartPosition(oc)
	if !ok {
		return nil
	}
	for _, o := range oc.Objects {
		se, ok := o.(*microflows.StartEvent)
		if !ok {
			continue
		}
		p := se.GetPosition()
		if p == derived {
			return nil
		}
		return &p
	}
	return nil
}

// startAnnotationLines returns the `@start(x, y)` line a description needs, or
// nothing at all.
//
// Emitted only for a start that is not where the layout would have put it, so an
// ordinary description does not grow a line restating its own arithmetic — and,
// more than cosmetics, so a described flow does not come back with its start
// pinned to a spot the next rewrite has to strand it at (#951). A start the
// arithmetic cannot reconstruct is emitted, which is what makes a described flow
// round-trip exactly (#884).
//
// The line goes ahead of the leading statement's own annotations, because the
// statement the start flows into is the one it belongs to — the same placement
// @merge uses for the other node with no statement of its own.
//
// Shared by both describers: formatMicroflowActivities and
// formatMicroflowActivitiesWithSourceMap are near-duplicates, and a describer
// that emits it while its twin does not makes the round-trip depend on which
// command the author happened to run.
func startAnnotationLines(oc *microflows.MicroflowObjectCollection) []string {
	p := authoredStartPosition(oc)
	if p == nil {
		return nil
	}
	return []string{fmt.Sprintf("@start(%d, %d)", p.X, p.Y)}
}
