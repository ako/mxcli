// SPDX-License-Identifier: Apache-2.0

// Package executor - parameter placement across a flow rewrite.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// positionFromAST converts an `@position(x, y)` written on a parameter into the
// semantic model's point. nil in, nil out: no annotation means the layout
// places the parameter, which is what microflows.DerivedParameterPosition does.
func positionFromAST(p *ast.Position) *model.Point {
	if p == nil {
		return nil
	}
	return &model.Point{X: p.X, Y: p.Y}
}

// parameterPositionAnnotation returns the `@position(x, y)` line a description
// needs for the parameter at index idx, or "" when the layout would put it
// exactly where it is.
//
// Emitting only an authored position is what keeps a described flow
// round-tripping: a line restating mxcli's own arithmetic would pin every
// parameter of every rewritten flow to the grid it happened to be on, so
// inserting a parameter would strand the others (the #951 lesson, which the
// StartEvent learned first — see authoredStartPosition).
//
// Readers already normalise, so Position is non-nil only when it is intent;
// the derived comparison here is belt-and-braces for a model assembled in
// memory rather than read from disk.
func parameterPositionAnnotation(p *microflows.MicroflowParameter, idx int, indent string) string {
	if p == nil || p.Position == nil {
		return ""
	}
	if *p.Position == microflows.DerivedParameterPosition(idx) {
		return ""
	}
	return fmt.Sprintf("%s@position(%d, %d)", indent, p.Position.X, p.Position.Y)
}

// describeMicroflowParameters renders the parenthesised parameter list of a
// flow header — one parameter per line, each preceded by its `@position` when
// it has one.
//
// Shared by all four describers (microflow, nanoflow, the generic flow
// describer and the rule describer). They were four copies of the same six
// lines, and a describer that emits the annotation while its twins do not makes
// the round-trip depend on which command the author happened to run — the same
// trap startAnnotationLines calls out.
func describeMicroflowParameters(params []*microflows.MicroflowParameter, formatType func(*microflows.MicroflowParameter) string) []string {
	lines := make([]string, 0, len(params)*2)
	for i, param := range params {
		if ann := parameterPositionAnnotation(param, i, "  "); ann != "" {
			lines = append(lines, ann)
		}
		paramType := "Object"
		if param.Type != nil {
			paramType = formatType(param)
		}
		comma := ","
		if i == len(params)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  $%s: %s%s", param.Name, paramType, comma))
	}
	return lines
}
