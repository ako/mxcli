// SPDX-License-Identifier: Apache-2.0

// MDL-WORKFLOW10: SET TASK OUTCOME on a user task the microflow never claimed.
//
// Completing a user task that is not assigned to the current user fails at
// RUNTIME, with nothing in either checker to warn you:
//
//	ERROR - Client: You can't complete this user task, it is not assigned to you.
//
// and the button simply does nothing. `mxcli check` passed, `mx check` passed,
// the build was clean, and the fault cost about eight minutes of runtime
// archaeology (ako/mxcli-maintenance-2).
//
// The trap is that TARGETING XPATH / TARGETING MICROFLOW decides who may SEE a
// task; it does not assign it. There is no ASSIGN TASK statement — claiming is a
// plain write to the Assignees association, and it has to come first:
//
//	change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
//	commit $Task;
//	set task outcome $Task 'Plan';
//
// A WARNING, not an error, and deliberately so: a task can legitimately be
// claimed somewhere this rule cannot see — in a microflow this one calls, in a
// nanoflow on the button, or by an earlier step in the process. Reporting those
// as errors would block correct apps. What the rule can say for certain is that
// THIS microflow completes a task it never assigned, which is the shape that
// fails.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// assigneesAssociation is the association a claim writes. Matched on the member
// name alone so every spelling of the qualified form is caught — quoted,
// unquoted, and with or without the System prefix, all of which parse.
const assigneesAssociation = "workflowusertask_assignees"

// ValidateTaskClaims reports SET TASK OUTCOME statements whose task was not
// assigned earlier in the same microflow.
func ValidateTaskClaims(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		out = append(out, taskClaimViolations(mf.Name, mf.Body)...)
	}
	return out
}

// taskClaimViolations walks one flow body in order, remembering which task
// variables have been claimed by the time each outcome is set.
//
// Order matters and is the point: claiming AFTER the outcome does not help, so
// the walk records claims as it passes them rather than collecting them all
// first.
func taskClaimViolations(name ast.QualifiedName, body []ast.MicroflowStatement) []linter.Violation {
	flowName := name.String()
	claimed := map[string]bool{}
	var out []linter.Violation

	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, st := range stmts {
			switch s := st.(type) {
			case *ast.ChangeObjectStmt:
				if changeClaimsTask(s) {
					claimed[strings.ToLower(s.Variable)] = true
				}
			case *ast.SetTaskOutcomeStmt:
				v := strings.ToLower(s.WorkflowTaskVariable)
				if claimed[v] {
					continue
				}
				out = append(out, linter.Violation{
					RuleID:   "MDL-WORKFLOW10",
					Severity: linter.SeverityWarning,
					Location: linter.Location{
						Module:       name.Module,
						DocumentType: "microflow",
						DocumentName: name.Name,
					},
					Message: fmt.Sprintf(
						"microflow %s: completes user task $%s without assigning it first — a user task "+
							"that is not assigned to the current user cannot be completed, and this fails at "+
							"RUNTIME (\"You can't complete this user task, it is not assigned to you\") with "+
							"the button appearing to do nothing. Neither mxcli check nor mx check catches it.",
						flowName, s.WorkflowTaskVariable),
					Suggestion: fmt.Sprintf(
						"Claim the task before setting the outcome:\n"+
							"    change $%s (System.WorkflowUserTask_Assignees = [%%CurrentUser%%]);\n"+
							"    commit $%s;\n"+
							"  TARGETING XPATH / TARGETING MICROFLOW decides who may SEE a task; it does not "+
							"assign it. Ignore this if the task is claimed elsewhere — in a microflow this one "+
							"calls, or earlier in the process.",
						s.WorkflowTaskVariable, s.WorkflowTaskVariable),
				})
			}
			// Nested bodies (loops, decisions, error handlers) can hold either half,
			// so a claim inside a branch still counts — being wrong in the quiet
			// direction is what keeps a warning worth reading.
			walk(nestedStatements(st))
		}
	}
	walk(body)
	return out
}

// changeClaimsTask reports whether a CHANGE writes the Assignees association.
func changeClaimsTask(s *ast.ChangeObjectStmt) bool {
	for _, c := range s.Changes {
		name := c.Attribute
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if strings.EqualFold(strings.Trim(name, `"`), assigneesAssociation) {
			return true
		}
	}
	return false
}

// nestedStatements returns the sub-bodies a control-flow statement holds.
//
// Only the containers a claim or an outcome can realistically sit in. A
// statement type missing here makes the rule quieter, never noisier, which is
// the right way for it to fail.
func nestedStatements(st ast.MicroflowStatement) []ast.MicroflowStatement {
	switch s := st.(type) {
	case *ast.IfStmt:
		return append(append([]ast.MicroflowStatement{}, s.ThenBody...), s.ElseBody...)
	case *ast.LoopStmt:
		return s.Body
	case *ast.WhileStmt:
		return s.Body
	}
	return nil
}
