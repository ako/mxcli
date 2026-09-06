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
//	Log                 ok       CE6035    CE6035
//	Create              ok       CE6035    CE6035
//	Change              ok       CE6035    CE6035
//	Commit              ok       CE6035    CE6035
//	Aggregate           ok       CE6035    CE6035
//
// A DENY-list rather than an allow-list, deliberately. The statements not named
// here were never measured, and refusing them would reject scripts that may well
// build — the rule reports what is known to fail and stays silent about the rest.
// That is also why the table is per-statement and not "activities that write to
// the database": Delete writes and is fine, Aggregate does not and is not.
//
// Only CREATE and COMMIT appear, because they are the only rejecting activities
// MDL can even put the clause on: `logStatement` and the change statement carry
// no onErrorClause in the grammar, so Log and Change are unreachable from a
// script however badly they behave in a stored document. Adding them would be
// dead code that reads as coverage.
var continueUnsupportedOn = map[string]string{
	"create": "Create object activity",
	"commit": "Commit object(s) activity",
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
	default:
		return "", ""
	}
	return keyword, continueUnsupportedOn[keyword]
}
