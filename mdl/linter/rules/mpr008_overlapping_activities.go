// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// activityBoxWidth and activityBoxHeight are the approximate pixel dimensions of a
// Mendix microflow activity box on the canvas. Two activities overlap when their
// top-left corner positions differ by less than these thresholds.
const activityBoxWidth = 120
const activityBoxHeight = 60

// OverlappingActivitiesRule flags microflow activities whose canvas positions overlap.
//
// The most common cause is writing multiple MDL statements after a single @position
// annotation — e.g. a DECLARE followed immediately by a SET with no second @position.
// The executor auto-places the un-annotated statement only 150px to the right (less
// than one activity width from the next explicitly annotated activity), producing
// overlapping boxes in Studio Pro.
type OverlappingActivitiesRule struct{}

func NewOverlappingActivitiesRule() *OverlappingActivitiesRule {
	return &OverlappingActivitiesRule{}
}

func (r *OverlappingActivitiesRule) ID() string                       { return "MPR008" }
func (r *OverlappingActivitiesRule) Name() string                     { return "OverlappingActivities" }
func (r *OverlappingActivitiesRule) Category() string                 { return "correctness" }
func (r *OverlappingActivitiesRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }
func (r *OverlappingActivitiesRule) Description() string {
	return "Microflow activities whose canvas positions overlap, typically caused by missing @position annotations in MDL"
}

func (r *OverlappingActivitiesRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	var violations []linter.Violation

	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) {
			continue
		}

		fullMF, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || fullMF == nil || fullMF.ObjectCollection == nil {
			continue
		}

		planes := overlapPlanes(fullMF.ObjectCollection.Objects)

		// Check all pairs for overlapping positions, WITHIN a plane.
		// Skip activities at the origin (0,0) — these are unpositioned/default.
		reported := make(map[string]bool)
		for _, activities := range planes {
			for i := 0; i < len(activities); i++ {
				for j := i + 1; j < len(activities); j++ {
					a, b := activities[i], activities[j]
					if (a.x == 0 && a.y == 0) || (b.x == 0 && b.y == 0) {
						continue
					}
					dx := a.x - b.x
					if dx < 0 {
						dx = -dx
					}
					dy := a.y - b.y
					if dy < 0 {
						dy = -dy
					}
					if dx < activityBoxWidth && dy < activityBoxHeight {
						key := fmt.Sprintf("%d,%d|%d,%d", a.x, a.y, b.x, b.y)
						if reported[key] {
							continue
						}
						reported[key] = true
						violations = append(violations, linter.Violation{
							RuleID:   r.ID(),
							Severity: r.DefaultSeverity(),
							Message: fmt.Sprintf(
								"Activities '%s' (%d,%d) and '%s' (%d,%d) overlap in microflow '%s.%s'. "+
									"Each MDL statement that creates a canvas activity needs its own @position annotation.",
								a.caption, a.x, a.y, b.caption, b.x, b.y,
								mf.ModuleName, mf.Name,
							),
							Location: linter.Location{
								Module:       mf.ModuleName,
								DocumentType: "microflow",
								DocumentName: mf.Name,
								DocumentID:   mf.ID,
							},
							Suggestion: "Add a separate @position(x, y) annotation before each statement. Use 190px spacing between activities.",
						})
					}
				}
			}
		}
	}

	return violations
}

// actInfo is one positioned node on a canvas.
type actInfo struct {
	x, y    int
	caption string
}

// overlapPlanes splits a microflow's objects into one plane per CANVAS, rather
// than flattening them into a single list.
//
// A LoopedActivity's children are positioned RELATIVE to the loop container,
// while everything on the microflow's own canvas is absolute. Flattening the two
// compares coordinates from different spaces and reports overlaps that cannot
// happen on screen — verified in the stored BSON: an outer activity at 200;230
// alongside a loop child at 141;130, where 141;130 is measured from the loop's
// own frame. A false "these overlap" is worse than silence here, because the
// point of the rule is to be trusted about positions. (upstream #884)
//
// The container itself belongs to its PARENT's plane; only its children get a
// new one.
func overlapPlanes(objects []microflows.MicroflowObject) [][]actInfo {
	var planes [][]actInfo
	var collect func(objs []microflows.MicroflowObject)
	collect = func(objs []microflows.MicroflowObject) {
		var plane []actInfo
		var nested [][]microflows.MicroflowObject
		for _, obj := range objs {
			switch act := obj.(type) {
			case *microflows.ActionActivity:
				caption := act.Caption
				if caption == "" {
					caption = "(unnamed)"
				}
				p := act.GetPosition()
				plane = append(plane, actInfo{p.X, p.Y, caption})
			case *microflows.LoopedActivity:
				caption := act.Caption
				if caption == "" {
					caption = "(loop)"
				}
				p := act.GetPosition()
				plane = append(plane, actInfo{p.X, p.Y, caption})
				if act.ObjectCollection != nil {
					nested = append(nested, act.ObjectCollection.Objects)
				}
			case *microflows.ExclusiveSplit:
				p := act.GetPosition()
				plane = append(plane, actInfo{p.X, p.Y, act.Caption})
			case *microflows.ExclusiveMerge:
				p := act.GetPosition()
				plane = append(plane, actInfo{p.X, p.Y, "(merge)"})
			}
		}
		planes = append(planes, plane)
		for _, o := range nested {
			collect(o)
		}
	}
	collect(objects)
	return planes
}
