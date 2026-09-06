// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/model"
)

// IrreducibleFlowGraphRule (MDL-FLOW01) flags microflows whose branch structure
// `DESCRIBE MICROFLOW` cannot render faithfully.
//
// MDL's `if/then/else` is a single-entry/single-exit block; a Mendix microflow is
// an arbitrary graph. When a branch re-enters a sibling branch's path the
// describer walks it as a tree anyway and emits MDL that means something else —
// silently. On the graph in mendixlabs/mxcli#923 the described program was the
// exact inverse of the original: it always logged, the description never did.
//
// The rule is INFO rather than a warning because the model itself is valid and
// builds cleanly. What is broken is mxcli's ability to describe it, so the
// message says that rather than implying the user did something wrong.
type IrreducibleFlowGraphRule struct{}

func NewIrreducibleFlowGraphRule() *IrreducibleFlowGraphRule {
	return &IrreducibleFlowGraphRule{}
}

func (r *IrreducibleFlowGraphRule) ID() string                       { return "MDL-FLOW01" }
func (r *IrreducibleFlowGraphRule) Name() string                     { return "IrreducibleFlowGraph" }
func (r *IrreducibleFlowGraphRule) Category() string                 { return "quality" }
func (r *IrreducibleFlowGraphRule) DefaultSeverity() linter.Severity { return linter.SeverityInfo }

func (r *IrreducibleFlowGraphRule) Description() string {
	return "Microflow branch structure cannot be described faithfully as nested if/then/else"
}

func (r *IrreducibleFlowGraphRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx.Reader() == nil {
		return nil
	}

	var violations []linter.Violation
	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) {
			continue
		}
		full, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || full == nil || full.ObjectCollection == nil {
			continue
		}
		oc := full.ObjectCollection
		for _, f := range microflowgraph.Analyze(oc.Objects, oc.Flows) {
			violations = append(violations, r.violation(mf, f))
		}
	}
	return violations
}

func (r *IrreducibleFlowGraphRule) violation(mf linter.Microflow, f microflowgraph.Finding) linter.Violation {
	where := ""
	if f.Split != nil {
		p := f.Split.GetPosition()
		where = fmt.Sprintf(" at (%d, %d)", p.X, p.Y)
	}

	var what, suggestion string
	switch f.Class {
	case microflowgraph.Recombinable:
		what = fmt.Sprintf(
			"%d branches of a decision%s rejoin at one shared point before the decision's own merge, "+
				"so the block cannot be nested. DESCRIBE renders it as nested IFs and drops the path that "+
				"enters the shared part early — the described flow is not the flow you drew",
			f.BranchCount, where)
		suggestion = "Describing this microflow does not round-trip. Edit it in Studio Pro, or restructure so " +
			"each branch reaches the decision's merge without passing through another branch's path. " +
			"The condition can usually be folded into one decision instead (e.g. `not(a) or b`)."
	default:
		what = fmt.Sprintf(
			"%d branches of a decision%s cross: they share %d entry points, not one",
			f.BranchCount, where, len(f.Entries))
		suggestion = "Describing this microflow does not round-trip, and the shape cannot be expressed as " +
			"nested IFs without duplicating an activity or adding a helper variable. Edit it in Studio Pro."
	}

	return linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("'%s.%s': %s [%s, overlap %d node(s)]",
			mf.ModuleName, mf.Name, what, f.Class, len(f.Overlap)),
		Location: linter.Location{
			Module:       mf.ModuleName,
			DocumentType: mf.DocumentNoun(),
			DocumentName: mf.Name,
			DocumentID:   mf.ID,
		},
		Suggestion: suggestion,
	}
}
