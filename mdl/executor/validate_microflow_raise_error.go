// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL084 — `raise error;` outside an error handler.
//
// An error event re-raises the error currently being handled, so Mendix allows
// one only where an error is in scope. From Mendix's own reference (error-event):
//
//	"You can only use an error event if an error is in scope: Studio Pro does not
//	 allow you to connect the normal execution flow to an error event, because
//	 there would not be an error to pass back to the caller."
//
// Studio Pro cannot draw the invalid shape at all; mxcli could, and did. The
// build rejects it:
//
//	[error] [CE0710] "The main flow cannot join an error flow or end in an error event"
//
// This is NOT the flow-graph wiring defect mendixlabs/mxcli#1030 diagnosed. That
// report blamed a trailing EndEvent appended because a "ends with return" flag was
// never set by RaiseErrorStmt's dispatch — but `isTerminalStmt` has treated
// RaiseErrorStmt as a terminator all along, and the document mxcli builds for
// `raise error;` is exactly the one the report asks for: a start event, an error
// event, one sequence flow, no trailing end event and no outgoing flow from the
// error event (pinned by TestMainFlowErrorEventIsTheShapeMxbuildRejects). Wiring
// it differently cannot help, because *any* main-flow error event is CE0710 —
// which is why the fix is a refusal and not a builder change.
//
// The rule lives with the #893 CE-gap rules and shares their posture: error
// severity, so `exec`'s pre-flight refuses the script with nothing written, and
// deliberately off execEnforcedMicroflowRules so `--no-check` remains an escape
// hatch for anyone who needs to reproduce the build failure.

// mainFlowRaiseErrors counts the `raise error;` statements reachable on a flow's
// MAIN path — every RaiseErrorStmt in the body, in branches, splits and loop
// bodies, but NOT inside an `on error … { … }` handler, where an error event is
// legal and expected.
//
// Skipping handler bodies is the whole content of the rule, so the walk cannot
// reuse the validator's walkBody, which descends into them on purpose (an
// ON ERROR body is still ordinary MDL for every other check). A handler nested
// inside a handler stays skipped: recursion simply does not enter either.
func mainFlowRaiseErrors(body []ast.MicroflowStatement) int {
	n := 0
	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.RaiseErrorStmt:
				n++
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.LoopStmt:
				walk(st.Body)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
			// Deliberately NOT walking stmtErrorHandling(s).Body — see above.
		}
	}
	walk(body)
	return n
}

// checkRaiseErrorOutsideHandler flags a main-flow `raise error;` — MDL084.
func (v *microflowValidator) checkRaiseErrorOutsideHandler(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() || mainFlowRaiseErrors(body) == 0 {
		return
	}
	v.addViolation("MDL084", linter.SeverityError,
		"`raise error;` outside an ON ERROR handler builds an error event on the main flow, "+
			"which Mendix does not allow — an error event re-raises the error being handled, so "+
			"one is only legal where an error is in scope. mxbuild rejects it with CE0710 \"The "+
			"main flow cannot join an error flow or end in an error event\", and Studio Pro will "+
			"not draw the shape at all.",
		"Put the `raise error;` inside the `on error { … }` handler of the activity whose failure "+
			"you are re-raising. To fail deliberately from the main flow, call a Java action that "+
			"throws — Mendix has no main-flow \"throw\" activity.")
}

// raiseErrorOutsideHandlerRuleError is the rule flavour of MDL084. A rule is
// built by the same flow builder, so a main-flow `raise error;` in one is the
// same CE0710; leaving rules out would ship the fixed defect in a sibling path,
// which is how MDL051 reached users covering `break` but not `continue` (#791).
func raiseErrorOutsideHandlerRuleError(body []ast.MicroflowStatement) string {
	if mainFlowRaiseErrors(body) == 0 {
		return ""
	}
	return "a rule cannot `raise error;` outside an ON ERROR handler — an error event on the " +
		"main flow is CE0710 \"The main flow cannot join an error flow or end in an error event\""
}
