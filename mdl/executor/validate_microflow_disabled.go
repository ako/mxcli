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

// FlowBodyUsesDisabled reports whether any statement in a flow body (at any
// nesting depth) carries `@disabled`.
//
// The version gate reads the STATEMENT rather than the built model so it can
// run before the build and name the annotation in its error. Bodies are walked
// through ast.StatementBodies, so a `@disabled` inside a loop or an if branch
// counts — the gate is about the document that would be written, and a nested
// activity is written into the same one.
func FlowBodyUsesDisabled(body []ast.MicroflowStatement) bool {
	for _, s := range body {
		if ann := ast.StatementAnnotations(s); ann != nil && ann.Disabled {
			return true
		}
		for _, nested := range ast.StatementBodies(s) {
			if FlowBodyUsesDisabled(nested) {
				return true
			}
		}
	}
	return false
}

// checkDisabledActivityFeature gates `@disabled` on the project's Mendix
// version. Microflows$ActionActivity.disabled was introduced in **9.12.0** —
// mendixmodelsdk 4.115.0's own StructureVersionInfo, which is the arbiter for a
// metamodel property, and modelsdk/gen/microflows/version.go agrees.
// sdk/versions/mendix-9.yaml's supported range starts at 9.0.0, so the window
// below the floor is real rather than theoretical.
//
// This is a gate with no downstream safety net, which is why it is worth having:
// mxbuild tolerates a property it does not know, so a pre-9.12 project carrying
// `Disabled` builds green and Studio Pro throws `Sequence contains no matching
// element` at MprProperty.cs — CLAUDE.md's overlay-writes rule, reached from the
// version axis instead of the spelling one.
func checkDisabledActivityFeature(ctx *ExecContext, body []ast.MicroflowStatement) error {
	if !FlowBodyUsesDisabled(body) {
		return nil
	}
	return checkFeature(ctx, "microflows", "disabled_activity", "@disabled on an activity",
		"Microflows$ActionActivity has no `disabled` property before Mendix 9.12 — "+
			"remove the annotation, or upgrade the project")
}
