// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateNanoflow runs the MDL0xx rules that hold for a nanoflow body — today
// only MDL044, an expression calling a name that is not a Mendix function.
//
// MDL044 was wired to CREATE MICROFLOW alone (#828), so `currentDeviceType()`
// in a nanoflow passed check and exec and failed the build with CE0117
// (mendixlabs/mxcli#1033). The microflow rule set is deliberately NOT run
// wholesale: several rules are microflow-specific and would be false positives
// here — MDL057 refuses `synchronize`, which only a nanoflow may contain.
func ValidateNanoflow(stmt *ast.CreateNanoflowStmt) []linter.Violation {
	v := &microflowValidator{
		mfName:     stmt.Name.String(),
		docType:    "nanoflow",
		returnType: stmt.ReturnType,
		varKinds:   map[string]exprcheck.TypeKind{},
	}
	v.walkExprFunctions(stmt.Body)
	return v.violations
}

// walkExprFunctions applies checkStmtExprFunctions to every statement in the
// body, including branches, loops and error-handler bodies.
func (v *microflowValidator) walkExprFunctions(body []ast.MicroflowStatement) {
	for _, s := range body {
		v.checkStmtExprFunctions(s)
		switch stmt := s.(type) {
		case *ast.IfStmt:
			v.walkExprFunctions(stmt.ThenBody)
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			for _, c := range stmt.Cases {
				v.walkExprFunctions(c.Body)
			}
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				v.walkExprFunctions(c.Body)
			}
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.LoopStmt:
			v.walkExprFunctions(stmt.Body)
		case *ast.WhileStmt:
			v.walkExprFunctions(stmt.Body)
		}
		if eh := stmtErrorHandling(s); eh != nil {
			v.walkExprFunctions(eh.Body)
		}
	}
}

// validateNanoflowRules is validateMicroflowRules for a nanoflow: exec refuses
// what ValidateNanoflow reports, restricted to the same verified allowlist, so
// check and exec cannot disagree.
func validateNanoflowRules(stmt *ast.CreateNanoflowStmt) error {
	var msgs []string
	for _, v := range ValidateNanoflow(stmt) {
		if v.Severity != linter.SeverityError || !execEnforcedMicroflowRules[v.RuleID] {
			continue
		}
		msg := fmt.Sprintf("[%s] %s", v.RuleID, v.Message)
		if v.Suggestion != "" {
			msg += "\n    " + v.Suggestion
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("nanoflow '%s' has validation errors:\n  - %s",
		stmt.Name.String(), strings.Join(msgs, "\n  - "))
}
