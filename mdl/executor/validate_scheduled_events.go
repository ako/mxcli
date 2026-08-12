// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for CREATE SCHEDULED EVENT.
//
// The repeat rule is a polymorphic child with eight variants that differ in
// which fields they carry, so most of what can go wrong — a field that belongs
// to a different repeat, an hour of 99, a misspelled weekday — is decidable from
// the statement alone. Running it here means a plain `mxcli check` reports it,
// instead of the script passing check and failing at exec.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateScheduledEvents reports (MDL-SCHED01) a CREATE SCHEDULED EVENT whose
// properties do not describe a schedule Mendix can store.
//
// It reuses the executor's own builder, so check and exec cannot drift: there is
// one implementation of what a valid statement is, and this pass is the same
// function exec calls before writing.
func ValidateScheduledEvents(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateScheduledEventStmt)
		if !ok {
			continue
		}
		if _, err := scheduledEventFromStmt(s); err != nil {
			out = append(out, linter.Violation{
				RuleID:   "MDL-SCHED01",
				Severity: linter.SeverityError,
				Message:  err.Error(),
				Suggestion: "Each Repeat takes only its own fields — see `mxcli syntax scheduled-event` " +
					"for the field list of each one.",
			})
		}
	}
	return out
}
