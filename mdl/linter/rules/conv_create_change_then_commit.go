// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// CreateChangeThenCommitRule flags a Create object or Change object activity
// that does not commit, directly followed by a Commit activity of that same
// object. The two activities do what one does: the create or change with its
// Commit option set (Yes, or Yes without events), which is how Studio Pro
// models it and what reviewers expect to see.
//
// It reports only a pair the merge would not change the meaning of:
//   - the create/change has Commit = No, and the commit names its object;
//   - the flow between them is the normal flow, not an error handler;
//   - the commit has no other incoming flow, so no other path relies on it;
//   - neither has custom error handling, which the merge would move or drop.
//
// Commit's "with events" maps onto Commit = Yes, "without events" onto Yes
// without events, and Refresh in client is kept if either had it. Microflows
// only, as asked for; a nanoflow's create/change offers the same option, but
// nothing measured that the pair behaves identically there.
type CreateChangeThenCommitRule struct{}

func NewCreateChangeThenCommitRule() *CreateChangeThenCommitRule {
	return &CreateChangeThenCommitRule{}
}

func (r *CreateChangeThenCommitRule) ID() string       { return "CONV021" }
func (r *CreateChangeThenCommitRule) Name() string     { return "CreateChangeThenCommit" }
func (r *CreateChangeThenCommitRule) Category() string { return "quality" }
func (r *CreateChangeThenCommitRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *CreateChangeThenCommitRule) Description() string {
	return "A Create or Change object activity directly followed by a Commit of the same object; set Commit on the create/change instead"
}

func (r *CreateChangeThenCommitRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx.Reader() == nil {
		return nil
	}
	var violations []linter.Violation
	for mf := range ctx.Microflows() {
		if mf.MicroflowType != "MICROFLOW" || ctx.IsExcluded(mf.ModuleName) {
			continue
		}
		full, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || full == nil || full.ObjectCollection == nil {
			continue
		}
		violations = append(violations, findCreateChangeThenCommit(full.ObjectCollection, mf, r)...)
	}
	return violations
}

// findCreateChangeThenCommit reports each create/change → commit pair in oc
// and in the bodies of its loops (a loop body has its own flows).
func findCreateChangeThenCommit(oc *microflows.MicroflowObjectCollection, mf linter.Microflow,
	r *CreateChangeThenCommitRule) []linter.Violation {

	byID := map[model.ID]microflows.MicroflowObject{}
	incoming := map[model.ID]int{}
	for _, obj := range oc.Objects {
		byID[obj.GetID()] = obj
	}
	for _, f := range oc.Flows {
		incoming[f.DestinationID]++
	}

	var violations []linter.Violation
	for _, f := range oc.Flows {
		if f.IsErrorHandler || incoming[f.DestinationID] != 1 {
			continue
		}
		from, ok := byID[f.OriginID].(*microflows.ActionActivity)
		if !ok {
			continue
		}
		to, ok := byID[f.DestinationID].(*microflows.ActionActivity)
		if !ok {
			continue
		}
		commit, ok := to.Action.(*microflows.CommitObjectsAction)
		if !ok || !defaultErrorHandling(to) || !defaultErrorHandling(from) {
			continue
		}
		kind, variable, refresh, ok := uncommittedCreateOrChange(from.Action)
		if !ok || variable == "" || !strings.EqualFold(variable, commit.CommitVariable) {
			continue
		}
		violations = append(violations, createChangeThenCommitViolation(mf, r, kind, variable, commit, refresh))
	}

	for _, obj := range oc.Objects {
		if loop, ok := obj.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
			violations = append(violations, findCreateChangeThenCommit(loop.ObjectCollection, mf, r)...)
		}
	}
	return violations
}

// uncommittedCreateOrChange returns the activity kind ("Create" or "Change"),
// the object variable it creates or changes, and its Refresh in client, when
// action is a create/change whose Commit is No. An empty Commit is unknown, not
// No, and is skipped.
func uncommittedCreateOrChange(action microflows.MicroflowAction) (kind, variable string, refresh, ok bool) {
	switch a := action.(type) {
	case *microflows.CreateObjectAction:
		if a.Commit == microflows.CommitTypeNo {
			return "Create", a.OutputVariable, a.RefreshInClient, true
		}
	case *microflows.ChangeObjectAction:
		if a.Commit == microflows.CommitTypeNo {
			return "Change", a.ChangeVariable, a.RefreshInClient, true
		}
	}
	return "", "", false, false
}

// defaultErrorHandling reports whether the activity's error handling is the
// default (Rollback), stored on the action or the activity, or left unset.
func defaultErrorHandling(a *microflows.ActionActivity) bool {
	t := microflows.ObjectErrorHandlingType(a)
	return t == "" || t == microflows.ErrorHandlingTypeRollback
}

func createChangeThenCommitViolation(mf linter.Microflow, r *CreateChangeThenCommitRule, kind, variable string,
	commit *microflows.CommitObjectsAction, refresh bool) linter.Violation {

	option, mdl := "Yes", "commit"
	if !commit.WithEvents {
		option, mdl = "Yes without events", "commit without events"
	}
	if refresh || commit.RefreshInClient {
		option += ", with Refresh in client"
		mdl += " refresh"
	}
	verb, statement := "changes", fmt.Sprintf("change $%s (…) %s;", variable, mdl)
	if kind == "Create" {
		verb, statement = "creates", fmt.Sprintf("$%s = create …(…) %s;", variable, mdl)
	}
	return linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("%s '%s.%s' %s $%s and commits it in the very next activity: "+
			"one %s object activity with Commit set does both.",
			mf.DocumentNounTitle(), mf.ModuleName, mf.Name, verb, variable, kind),
		Location: linter.Location{
			Module:       mf.ModuleName,
			DocumentType: mf.DocumentNoun(),
			DocumentName: mf.Name,
			DocumentID:   mf.ID,
		},
		Suggestion: fmt.Sprintf("Remove the Commit activity and set Commit to %s on the %s object activity (in MDL: %s)",
			option, kind, statement),
	}
}
