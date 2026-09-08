// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for workflows. Workflows previously
// received no semantic validation at all (CreateWorkflowStmt has no case in
// validateWithContext), so several constructs passed `mxcli check` but were
// rejected by MxBuild, each costing a build round-trip. These heuristics catch
// the syntax-only cases up front. See
// docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md.
package executor

import (
	"fmt"
	"regexp"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// wfOutcomeIdentRe matches a valid Mendix EnumerationValueIdentifier: dotted
// identifier segments, no spaces or other punctuation. Decision /
// call-microflow outcome names must be enum value identifiers; free text like
// 'Confirmed closed' is rejected by MxBuild.
//
// The qualified form is what Studio Pro actually stores — every
// EnumerationValueConditionOutcome in the demo corpus holds
// Module.Enum.Value (7 of 7 non-empty), so `describe workflow` emits it and a
// bare-identifier-only rule refused mxcli's own output (ako/mxcli#408). Bare
// values stay accepted: the rule's job is to catch free text, not to pick a
// spelling.
var wfOutcomeIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// ValidateWorkflow checks a workflow for constructs that pass parsing but are
// rejected by MxBuild, without requiring a project connection.
//
//   - MDL-WF01: user task without a page (CE1834)
//   - MDL-WF02: single-outcome user task containing nested activities (CE1876)
//   - MDL-WF03: decision / call-microflow outcome that is not a valid
//     enumeration value identifier
//   - MDL-WF06: enumeration outcomes with no empty-valued branch (CE6686)
//   - MDL-WF04: standalone `annotation` in a workflow body (unloadable model)
//   - MDL-WF05: `jump to` a target that names no activity (see validate_workflow_jump.go)
func ValidateWorkflow(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	var out []linter.Violation
	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}
	walkWorkflowActivities(stmt.Activities, func(a ast.WorkflowActivityNode) {
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			label := workflowUserTaskLabel(n)
			// MDL-WF01 — user task without a page.
			if n.Page.Module == "" && n.Page.Name == "" {
				out = append(out, linter.Violation{
					RuleID:     "MDL-WF01",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("user task %s has no page — MxBuild requires the Page property (CE1834)", label),
					Suggestion: "Add a page typed to System.WorkflowUserTask, e.g. `page Module.TaskPage`.",
				})
			}
			// MDL-WF02 — a single outcome must not carry a nested flow.
			if len(n.Outcomes) == 1 && len(n.Outcomes[0].Activities) > 0 {
				out = append(out, linter.Violation{
					RuleID:     "MDL-WF02",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("user task %s has a single outcome with nested activities — MxBuild rejects this (CE1876)", label),
					Suggestion: "Move the activities to the workflow's main flow after the user task, or add a second outcome.",
				})
			}
		case *ast.WorkflowDecisionNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "decision", loc)...)
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "decision", workflowDecisionLabel(n), loc)...)
		case *ast.WorkflowCallMicroflowNode:
			out = append(out, checkWorkflowOutcomeNames(n.Outcomes, "call microflow", loc)...)
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "call microflow", workflowCallMicroflowLabel(n), loc)...)
		case *ast.WorkflowAnnotationActivityNode:
			// MDL-WF04 — a standalone annotation is written into the workflow's
			// activity flow, but Mendix constructs every child of that list with a
			// Flow parent, and no annotation type takes one. The result is not a
			// build error but a project Mendix cannot LOAD, so Studio Pro will not
			// open it and `mx check` dies before validating anything.
			out = append(out, linter.Violation{
				RuleID:     "MDL-WF04",
				Severity:   linter.SeverityError,
				Location:   loc,
				Message:    "a standalone `annotation` in a workflow body produces a model Mendix cannot load (the annotation is placed in the activity flow, which accepts only flow elements) — Studio Pro will not open the project",
				Suggestion: "Remove the `annotation` statement. Use an MDL comment (`-- ...`) to keep the note in the script; workflow canvas annotations are not yet writable.",
			})
		}
	})
	out = append(out, ValidateWorkflowJumpTargets(stmt)...)
	return out
}

// ValidateAlterWorkflow applies the outcome-set rule to the activities an ALTER
// WORKFLOW statement introduces (MDL-WF06).
//
// ALTER reaches the same build error as CREATE: an `INSERT AFTER … decision`
// whose outcomes name enumeration values but no empty one is CE6686, measured on
// mxbuild 11.10.0 against a project that was at 0 errors before the ALTER. The
// activity-shaped ops carry an ordinary WorkflowActivityNode, so the check is
// the CREATE one over the introduced subtree — the same shape
// validateAlterWorkflowRefs uses for references.
//
// Only MDL-WF06 runs here. The others are not simply un-ported: MDL-WF01/WF02
// describe a state a later op in the same script can still repair (`SET ACTIVITY
// … PAGE`), and MDL-WF05 resolves jump targets against activities the statement
// cannot see. The outcome set of an inserted activity is complete where it is
// written — `INSERT OUTCOME` cannot extend it, since on a decision it writes a
// UserTaskOutcome into a ConditionOutcome list and yields a model Mendix cannot
// load at all.
func ValidateAlterWorkflow(stmt *ast.AlterWorkflowStmt) []linter.Violation {
	loc := linter.Location{
		Module:       stmt.Name.Module,
		DocumentType: "workflow",
		DocumentName: stmt.Name.Name,
	}
	var added []ast.WorkflowActivityNode
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			added = append(added, o.NewActivity)
		case *ast.ReplaceActivityOp:
			added = append(added, o.NewActivity)
		case *ast.InsertOutcomeOp:
			added = append(added, o.Activities...)
		case *ast.InsertPathOp:
			added = append(added, o.Activities...)
		case *ast.InsertBranchOp:
			added = append(added, o.Activities...)
		case *ast.InsertBoundaryEventOp:
			added = append(added, o.Activities...)
		}
	}
	var out []linter.Violation
	walkWorkflowActivities(added, func(a ast.WorkflowActivityNode) {
		switch n := a.(type) {
		case *ast.WorkflowDecisionNode:
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "decision", workflowDecisionLabel(n), loc)...)
		case *ast.WorkflowCallMicroflowNode:
			out = append(out, checkWorkflowEmptyEnumOutcome(n.Outcomes, "call microflow", workflowCallMicroflowLabel(n), loc)...)
		}
	})
	return out
}

// checkWorkflowOutcomeNames flags condition-outcome values (decision / call
// microflow branches) that are not valid enumeration value identifiers (MDL-WF03).
func checkWorkflowOutcomeNames(outcomes []ast.WorkflowConditionOutcomeNode, kind string, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	for _, o := range outcomes {
		if o.Value == "" || wfOutcomeIdentRe.MatchString(o.Value) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:     "MDL-WF03",
			Severity:   linter.SeverityError,
			Location:   loc,
			Message:    fmt.Sprintf("%s outcome '%s' is not a valid enumeration value identifier — MxBuild rejects outcome names with spaces or punctuation", kind, o.Value),
			Suggestion: "Use an enumeration value identifier — bare ('ConfirmedClosed') or qualified ('Module.Enum.ConfirmedClosed'); a decision branches on the enumeration returned by its expression, so outcome names must match that enum's value identifiers.",
		})
	}
	return out
}

// checkWorkflowEmptyEnumOutcome flags an activity that branches on an
// enumeration without an outcome for the EMPTY value (MDL-WF06).
//
// Mendix generates one outcome per enumeration value **plus one with an empty
// value**, and mxbuild compares the stored set against that generated set:
// anything else is CE6686 ("The current outcomes of the ... do not match the
// configured expression/microflow. Regenerate the outcomes."). Studio Pro's own
// documents agree — every enum decision in the FactoryManagement demo app
// stores the extra Workflows$EnumerationValueConditionOutcome with Value ”.
//
// Measured on mxbuild 11.10.0, in a blank 11.10.0 app:
//
//   - decision on `$WorkflowContext/Kind` with the two enum values → 1 error,
//     CE6686; adding `” -> { }` → 0 errors.
//   - the same decision on an attribute carrying a REQUIRED (not null)
//     validation rule → still CE6686. The empty outcome is not about whether
//     the value can be empty in practice, which is why this is an error rather
//     than a warning.
//   - a call-microflow activity branching on an enumeration-returning microflow
//     → the same CE6686 ("...of the call microflow activity do not match the
//     configured microflow"), cleared the same way. Hence both call sites.
//   - a boolean decision (true/false) is 0 errors with no empty outcome, so the
//     rule must classify outcomes exactly as buildConditionOutcome does.
//
// mxbuild wants set EQUALITY, so a missing enumeration *value* is CE6686 too
// (measured: Standard + ” on a two-value enum is 1 error). That half needs the
// enumeration's definition and so belongs to the reference pass, not here; this
// rule reports only what is decidable from the statement alone.
func checkWorkflowEmptyEnumOutcome(outcomes []ast.WorkflowConditionOutcomeNode, kind, label string, loc linter.Location) []linter.Violation {
	var enumValues []string
	for _, o := range outcomes {
		switch o.Value {
		case "True", "False":
			// A boolean branch: buildConditionOutcome emits a
			// BooleanConditionOutcome and mxbuild wants exactly true/false.
			return nil
		case "Default":
			// A VoidConditionOutcome — not an enumeration branch.
			continue
		case "":
			// The empty-valued enumeration outcome this rule is about.
			return nil
		default:
			enumValues = append(enumValues, o.Value)
		}
	}
	if len(enumValues) == 0 {
		return nil
	}
	// Quote the CE6686 text MxBuild actually prints for this activity kind, so
	// searching the build output for it lands here.
	ceText := "the current outcomes of the decision activity do not match the configured expression"
	if kind == "call microflow" {
		ceText = "the current outcomes of the call microflow activity do not match the configured microflow"
	}
	return []linter.Violation{{
		RuleID:   "MDL-WF06",
		Severity: linter.SeverityError,
		Location: loc,
		Message: fmt.Sprintf(
			"%s %s branches on an enumeration but has no outcome for the empty value — MxBuild rejects this (CE6686 %q)",
			kind, label, ceText),
		Suggestion: "Add an empty outcome alongside the named values: `'' -> { }`. Mendix generates one outcome per enumeration value plus one for the empty value, and the stored set must match — a required (not null) attribute does not exempt it.",
	}}
}

// workflowDecisionLabel returns a human-readable label for a decision.
func workflowDecisionLabel(n *ast.WorkflowDecisionNode) string {
	switch {
	case n.Name != "":
		return "'" + n.Name + "'"
	case n.Caption != "":
		return "'" + n.Caption + "'"
	case n.Expression != "":
		return "on '" + n.Expression + "'"
	}
	return "(unnamed)"
}

// workflowCallMicroflowLabel returns a human-readable label for a call-microflow
// activity.
func workflowCallMicroflowLabel(n *ast.WorkflowCallMicroflowNode) string {
	if n.Name != "" {
		return "'" + n.Name + "'"
	}
	if qn := n.Microflow.String(); qn != "" && qn != "." {
		return "'" + qn + "'"
	}
	return "(unnamed)"
}

// workflowUserTaskLabel returns a human-readable label for a user task.
func workflowUserTaskLabel(n *ast.WorkflowUserTaskNode) string {
	if n.Name != "" {
		return "'" + n.Name + "'"
	}
	if n.Caption != "" {
		return "'" + n.Caption + "'"
	}
	return "(unnamed)"
}

// walkWorkflowActivities visits every activity node in a workflow, recursing
// into all nested activity flows (outcomes, parallel-split paths, boundary
// events).
func walkWorkflowActivities(acts []ast.WorkflowActivityNode, visit func(ast.WorkflowActivityNode)) {
	for _, a := range acts {
		if a == nil {
			continue
		}
		visit(a)
		switch n := a.(type) {
		case *ast.WorkflowUserTaskNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		case *ast.WorkflowDecisionNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
		case *ast.WorkflowCallMicroflowNode:
			for _, o := range n.Outcomes {
				walkWorkflowActivities(o.Activities, visit)
			}
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		case *ast.WorkflowParallelSplitNode:
			for _, p := range n.Paths {
				walkWorkflowActivities(p.Activities, visit)
			}
		case *ast.WorkflowWaitForNotificationNode:
			for _, b := range n.BoundaryEvents {
				walkWorkflowActivities(b.Activities, visit)
			}
		}
	}
}
