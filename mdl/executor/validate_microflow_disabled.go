// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL087 — `@disabled` on a statement that is not an action activity.
//
// `Disabled` is a property of Microflows$ActionActivity and of nothing else: it
// occurs exactly once in generated/metamodel, on that one type. So a split, a
// loop, a merge, an end event, a break, a continue and an error event have
// nowhere to store it, and the flow builder — which only ever reaches for the
// flag inside its `case *microflows.ActionActivity` arm — drops it.
//
// An error rather than a warning, for the same reason MDL059 is one: the point
// of the annotation is to make a step inert, and a script that says so and is
// silently ignored ships an ACTIVE step the author believed was off. That is
// worse than a refusal, and the refusal has an obvious remedy — the statements
// listed below all have bodies or branches whose contents CAN be disabled.
//
// Deliberately a list of AST types rather than a "not in the allow-list" test:
// the vocabulary grows by statement type, and a new statement is far more
// likely to be an action activity (most are) than one of these nine. A wrong
// guess in that direction costs nothing; the opposite would refuse a legal
// script. TestDisabledAnnotationTargetsAreNotActionActivities pins every entry
// against what the flow builder actually creates.
func (v *microflowValidator) checkDisabledAnnotationTarget(s ast.MicroflowStatement) {
	ann := ast.StatementAnnotations(s)
	if ann == nil || !ann.Disabled {
		return
	}
	kind, becomes, ok := nonDisablableStatement(s)
	if !ok {
		return
	}
	v.addViolation("MDL087", linter.SeverityError,
		fmt.Sprintf("`@disabled` has no effect on %s — it becomes %s, and Mendix stores the "+
			"flag on Microflows$ActionActivity and on no other microflow object, so the "+
			"annotation is dropped and the flow stays exactly as it was", kind, becomes),
		"Only an action activity can be disabled — a call, a create/change/delete, a "+
			"retrieve, a log, an aggregate, a list operation. Put `@disabled` on one of "+
			"those, or remove the statement instead of disabling it.")
}

// nonDisablableStatement names a statement and the microflow object it becomes,
// for the statements whose object is not an ActionActivity.
func nonDisablableStatement(s ast.MicroflowStatement) (kind, becomes string, ok bool) {
	switch s.(type) {
	case *ast.IfStmt:
		return "an IF", "a decision", true
	case *ast.EnumSplitStmt:
		return "a CASE", "a decision", true
	case *ast.InheritanceSplitStmt:
		return "a SPLIT TYPE", "an object type decision", true
	case *ast.LoopStmt:
		return "a LOOP", "a loop activity", true
	case *ast.WhileStmt:
		return "a WHILE", "a loop activity", true
	case *ast.MergeStmt:
		return "a MERGE", "a merge", true
	case *ast.JoinStmt:
		return "a JOIN", "a merge", true
	case *ast.ReturnStmt:
		return "a RETURN", "an end event", true
	case *ast.RaiseErrorStmt:
		return "a RAISE ERROR", "an error event", true
	case *ast.BreakStmt:
		return "a BREAK", "a break event", true
	case *ast.ContinueStmt:
		return "a CONTINUE", "a continue event", true
	}
	return "", "", false
}
