// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// continueUnsupportedRule flags `ON ERROR CONTINUE` on an activity Mendix does
// not offer it for.
const continueUnsupportedRule = "MDL076"

// continueUnsupportedOn names the statements whose stored activity rejects
// ErrorHandlingType "Continue", by the caption mxbuild uses in the error.
//
// MEASURED on Mendix 11.14.0, not inferred: each value was written into a stored
// document of an otherwise-clean project and the project built. The split is not
// guessable from the metamodel — one enum covers every action type, and mxbuild
// enforces a per-type subset:
//
//	action           Rollback   Continue   Abort
//	Retrieve            ok         ok        ok
//	Delete              ok         ok        ok
//	MicroflowCall       ok         ok      CE6035
//	CreateVariable      ok         ok         -
//	ChangeVariable      ok         ok         -
//	Log                 ok       CE6035    CE6035
//	Create              ok       CE6035    CE6035
//	Change              ok       CE6035    CE6035
//	Commit              ok       CE6035    CE6035
//	Aggregate           ok       CE6035    CE6035
//	ShowPage            ok       CE6035      -
//	ClosePage           ok       CE6035      -
//	ShowMessage         ok       CE6035      -
//	ValidationFeedback  ok       CE6035      -
//
// A DENY-list rather than an allow-list, deliberately. The statements not named
// here were never measured, and refusing them would reject scripts that may well
// build — the rule reports what is known to fail and stays silent about the rest.
// That is also why the table is per-statement and not "activities that write to
// the database": Delete writes and is fine, Aggregate does not and is not.
//
// The bottom nine rows became REACHABLE with mendixlabs/mxcli#1078, which gave
// eight more statements an onErrorClause so a Studio Pro error handler could
// survive DESCRIBE. Before that they were unreachable from a script and this list
// held only create and commit. Note the split it exposes, which no rule of thumb
// predicts: create-VARIABLE and change-VARIABLE accept Continue while
// change-OBJECT does not — measured on 11.14.0, one microflow per row.
var continueUnsupportedOn = map[string]string{
	"create":              "Create object activity",
	"commit":              "Commit object(s) activity",
	"change":              "Change object activity",
	"log":                 "Log message activity",
	"show page":           "Show page activity",
	"close page":          "Close page activity",
	"show message":        "Show message activity",
	"validation feedback": "Validation feedback activity",
}

// checkErrorHandlingContinueSupported reports `ON ERROR CONTINUE` on a statement
// whose activity Mendix rejects it for.
//
// This one is not a check GAP being closed — mxcli already wrote `Continue` into
// these activities, so the model it produced was one mxbuild refuses:
//
//	$T = CREATE Eh.Thing ( Code = 'a' ) COMMIT ON ERROR CONTINUE;
//	mx check -> [CE6035] "Error handling type is not supported"
//	              at Create object activity 'Create Thing (Code)'
//
// Refused rather than silently downgraded to Rollback. The two mean different
// things at runtime — Continue swallows the error and carries on, Rollback aborts
// the flow — and an author who wrote the clause meant the first; quietly writing
// the second gives a microflow that behaves differently from what its source
// says, which is the failure mode this whole area already had once
// (ako/CapTrackV3 FINDINGS §11).
func (v *microflowValidator) checkErrorHandlingContinueSupported(stmt ast.MicroflowStatement) {
	eh := stmtErrorHandling(stmt)
	if eh == nil || eh.Type != ast.ErrorHandlingContinue {
		return
	}
	keyword, activity := continueUnsupportedStatement(stmt)
	if activity == "" {
		return
	}
	v.addViolation(continueUnsupportedRule, linter.SeverityError,
		fmt.Sprintf("`%s ... on error continue` is not supported — Mendix rejects Continue error handling "+
			"on a %s with CE6035 \"Error handling type is not supported\"", keyword, activity),
		fmt.Sprintf("Drop the clause to keep Mendix's default (rollback and abort), or wrap the statement in a "+
			"custom handler, which %s does accept: `%s ... on error { ... }`. Continue IS supported on "+
			"`retrieve`, `delete` and `call microflow` — measured on 11.14.0.", activity, keyword))
}

// continueUnsupportedStatement returns the MDL keyword and the mxbuild activity
// caption for a statement whose activity rejects Continue, or "" for one that
// accepts it or was never measured.
func continueUnsupportedStatement(stmt ast.MicroflowStatement) (keyword, activity string) {
	switch stmt.(type) {
	case *ast.CreateObjectStmt:
		keyword = "create"
	case *ast.MfCommitStmt:
		keyword = "commit"
	case *ast.ChangeObjectStmt:
		keyword = "change"
	case *ast.LogStmt:
		keyword = "log"
	case *ast.ShowPageStmt:
		keyword = "show page"
	case *ast.ClosePageStmt:
		keyword = "close page"
	case *ast.ShowMessageStmt:
		keyword = "show message"
	case *ast.ValidationFeedbackStmt:
		keyword = "validation feedback"
	default:
		// DeclareStmt and MfSetStmt are deliberately absent: create-variable and
		// change-variable accept Continue on 11.14.0.
		return "", ""
	}
	return keyword, continueUnsupportedOn[keyword]
}

// errorHandlingUnavailableRule flags an ON ERROR clause on a statement whose
// Mendix activity has no ErrorHandlingType property at all.
const errorHandlingUnavailableRule = "MDL077"

// checkErrorHandlingSupported reports an ON ERROR clause the stored activity
// cannot hold.
//
// The `set` statement is overloaded: `$x = $y` is a Change variable activity,
// which HAS an ErrorHandlingType, while `$x = head($list)` and `$x = count($list)`
// are List operation and Aggregate activities, which do not — the property is
// absent from Microflows$ListOperationsAction and Microflows$AggregateAction in
// the metamodel, not merely unset. One MDL statement form therefore spans
// activities that can and cannot carry the clause.
//
// Refused rather than dropped. #1078 exists because an error handler that
// disappears between the model and the script is invisible until someone
// re-executes the script and finds the handler gone; accepting a clause here and
// writing nothing would rebuild that same trap one statement over.
func (v *microflowValidator) checkErrorHandlingSupported(stmt ast.MicroflowStatement) {
	if stmtErrorHandling(stmt) == nil {
		return
	}
	var form, activity string
	switch stmt.(type) {
	case *ast.ListOperationStmt:
		form, activity = "a list operation", "List operation activity"
	case *ast.AggregateListStmt:
		form, activity = "an aggregate", "Aggregate list activity"
	default:
		return
	}
	v.addViolation(errorHandlingUnavailableRule, linter.SeverityError,
		fmt.Sprintf("`on error` is not available on %s — Mendix's %s has no error-handling "+
			"property, so the clause could only be discarded", form, activity),
		"Drop the clause. To handle a failure around it, put the statement inside the "+
			"custom handler of an activity that does support one, or split the expression "+
			"out into a plain `set` (a Change variable activity), which does.")
}
