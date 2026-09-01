// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// LoopChildContainmentRule flags a microflow activity that lies outside the loop
// container holding it.
//
// This is the only automated check for the condition. The Mendix model carries no
// geometry rules, so a loop whose children sit far outside its box passes
// `mx check` with zero errors and builds and runs normally — it is simply drawn
// wrong when the flow is opened. That is what let upstream #884 problem 1 survive:
// the container's Size was computed from a statement COUNT before the body was
// built, so children at x=1500/2000 lived in a 480-wide box and nothing said so.
//
// mxcli no longer produces that (the box is now sized from the real child bounding
// box), but the rule catches it however it arrives — a project written by an older
// mxcli, hand-edited in Studio Pro, or a future layout change that reintroduces it.
type LoopChildContainmentRule struct{}

func NewLoopChildContainmentRule() *LoopChildContainmentRule {
	return &LoopChildContainmentRule{}
}

func (r *LoopChildContainmentRule) ID() string                       { return "MPR011" }
func (r *LoopChildContainmentRule) Name() string                     { return "LoopChildContainment" }
func (r *LoopChildContainmentRule) Category() string                 { return "correctness" }
func (r *LoopChildContainmentRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }
func (r *LoopChildContainmentRule) Description() string {
	return "Microflow activities positioned outside the loop container that holds them, which renders wrong in Studio Pro but passes mx check"
}

func (r *LoopChildContainmentRule) Check(ctx *linter.LintContext) []linter.Violation {
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
		for _, e := range escapedLoopChildren(fullMF.ObjectCollection.Objects) {
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message: fmt.Sprintf(
					"Activity '%s' at (%d,%d) lies outside the loop '%s' that contains it (box %dx%d) "+
						"in %s '%s.%s'. The flow renders wrong in Studio Pro; mx check does not detect this.",
					e.Child, e.ChildX, e.ChildY, e.Loop, e.BoxW, e.BoxH,
					mf.DocumentNoun(), mf.ModuleName, mf.Name),
				Location: linter.Location{
					Module:       mf.ModuleName,
					DocumentType: mf.DocumentNoun(),
					DocumentName: mf.Name,
					DocumentID:   mf.ID,
				},
				Suggestion: "Give the activity an @position inside the container, or remove a hand-set @position so the layout engine places it. " +
					"A loop's box is sized from its children, so a contained @position widens the box rather than escaping it.",
			})
		}
	}
	return violations
}

// escapee is one child positioned outside the container that holds it.
type escapee struct {
	Loop           string
	Child          string
	ChildX, ChildY int
	BoxW, BoxH     int
}

// escapedLoopChildren walks every LoopedActivity at any depth and returns the
// children whose boxes are not fully inside their container.
//
// A child's coordinates are relative to its OWN container, so each loop is
// checked in its own space and never against an ancestor's — the same
// distinction MPR008 needed after it was found comparing a loop child against
// the microflow's absolute canvas (#884).
func escapedLoopChildren(objects []microflows.MicroflowObject) []escapee {
	var out []escapee
	for _, obj := range objects {
		loop, ok := obj.(*microflows.LoopedActivity)
		if !ok || loop.ObjectCollection == nil {
			continue
		}
		boxW, boxH := loop.Size.Width, loop.Size.Height
		for _, child := range loop.ObjectCollection.Objects {
			if child == nil {
				continue
			}
			p := child.GetPosition()
			// (0,0) is unpositioned, not escaped — MPR008 skips these for the
			// same reason, and flagging them would bury the real cases.
			if p.X == 0 && p.Y == 0 {
				continue
			}
			var sz model.Size
			if withSize, ok := child.(interface{ GetSize() model.Size }); ok {
				sz = withSize.GetSize()
			}
			if boxW <= 0 || boxH <= 0 {
				continue
			}
			left, top := p.X-sz.Width/2, p.Y-sz.Height/2
			right, bottom := p.X+sz.Width/2, p.Y+sz.Height/2
			if left < 0 || top < 0 || right > boxW || bottom > boxH {
				out = append(out, escapee{
					Loop:   captionOf(loop, "loop"),
					Child:  captionOf(child, "(unnamed)"),
					ChildX: p.X, ChildY: p.Y,
					BoxW: boxW, BoxH: boxH,
				})
			}
		}
		// Nested containers are checked in their own coordinate space.
		out = append(out, escapedLoopChildren(loop.ObjectCollection.Objects)...)
	}
	return out
}

// captionOf names a canvas object for the message. Caption lives on the concrete
// types rather than the MicroflowObject interface, so this switches the way
// MPR008 does.
func captionOf(o microflows.MicroflowObject, fallback string) string {
	var c string
	switch act := o.(type) {
	case *microflows.ActionActivity:
		c = act.Caption
	case *microflows.LoopedActivity:
		c = act.Caption
	case *microflows.ExclusiveSplit:
		c = act.Caption
	case *microflows.ExclusiveMerge:
		c = "(merge)"
	}
	if c == "" {
		return fallback
	}
	return c
}
