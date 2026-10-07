// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// NoCommitInLoopRule flags commit actions inside loops, which cause N+1 database operations.
type NoCommitInLoopRule struct{}

func NewNoCommitInLoopRule() *NoCommitInLoopRule { return &NoCommitInLoopRule{} }

func (r *NoCommitInLoopRule) ID() string                       { return "CONV011" }
func (r *NoCommitInLoopRule) Name() string                     { return "NoCommitInLoop" }
func (r *NoCommitInLoopRule) Category() string                 { return "performance" }
func (r *NoCommitInLoopRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *NoCommitInLoopRule) Description() string {
	return "Commits should not be inside loops, whether a commit action or a create/change with commit (N+1 performance issue)"
}

func (r *NoCommitInLoopRule) Check(ctx *linter.LintContext) []linter.Violation {
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

		findCommitsInLoops(fullMF.ObjectCollection.Objects, mf, r, &violations, false)
	}

	return violations
}

func findCommitsInLoops(objects []microflows.MicroflowObject, mf linter.Microflow, r *NoCommitInLoopRule, violations *[]linter.Violation, insideLoop bool) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			if !insideLoop || act.Action == nil {
				continue
			}
			if what := committingActionKind(act.Action); what != "" {
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("%s '%s.%s' has %s inside a loop. "+
						"This causes N+1 database operations.",
						mf.DocumentNounTitle(), mf.ModuleName, mf.Name, what),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: mf.DocumentNoun(),
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Move the commit outside the loop, or collect objects in a list and commit once after the loop",
				})
			}
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				findCommitsInLoops(act.ObjectCollection.Objects, mf, r, violations, true)
			}
		}
	}
}

// committingActionKind describes an action that commits to the database, or
// returns "" for one that does not. A create or change activity with Commit set
// is one round trip per iteration exactly as a separate commit is — Studio Pro's
// recommender flags both as MXP004 (mendixlabs/mxcli#1217). Any value other than
// No commits: YesWithoutEvents skips the event handlers, not the database.
func committingActionKind(action microflows.MicroflowAction) string {
	switch a := action.(type) {
	case *microflows.CommitObjectsAction:
		return "a Commit action"
	case *microflows.ChangeObjectAction:
		if commits(a.Commit) {
			return "a Change object action with Commit enabled"
		}
	case *microflows.CreateObjectAction:
		if commits(a.Commit) {
			return "a Create object action with Commit enabled"
		}
	}
	return ""
}

func commits(c microflows.CommitType) bool {
	return c != "" && c != microflows.CommitTypeNo
}
