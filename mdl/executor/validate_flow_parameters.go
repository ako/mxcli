// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateFlowParameterAnnotations refuses an annotation written on a flow
// parameter that mxcli does not implement there.
//
// Same reasoning as checkUnknownAnnotations one node family over (#884):
// `@position` is the only canvas property a parameter has, so `@postion(300,
// 100)` or `@size(30, 30)` on one would parse, do nothing, and discard exactly
// the placement the author was trying to state — which is the whole reason
// someone writes the annotation at all (#993).
//
// A free function rather than a microflowValidator method because the three
// flow flavours do not share a validator: ValidateMicroflow takes a
// *ast.CreateMicroflowStmt, so a check living there would leave nanoflows and
// rules — which share the parameter grammar — unguarded.
func ValidateFlowParameterAnnotations(flow string, params []ast.MicroflowParam) []linter.Violation {
	var out []linter.Violation
	for _, p := range params {
		for _, name := range p.UnknownAnnotations {
			out = append(out, linter.Violation{
				RuleID:   "MDL059",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s: unknown annotation `@%s` on parameter `$%s` — it parses "+
					"but does nothing, so whatever it was meant to express is silently lost",
					flow, name, p.Name),
				Suggestion: fmt.Sprintf("`@position(x, y)` is the only annotation a parameter takes; "+
					"it places the parameter box on the canvas. If `@%s` is a typo of it, correct it.", name),
			})
		}
	}
	return out
}
