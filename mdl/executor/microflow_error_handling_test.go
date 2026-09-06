// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/CapTrackV3 FINDINGS §11. `ON ERROR CONTINUE` on a retrieve "did not help",
// and `describe microflow` showed the retrieve with no error handler — the clause
// parsed, executed without complaint, and vanished.
//
// It vanished at the last hop. The visitor builds the clause, and the flow
// builder sets it on the ACTIVITY — a field the metamodel does not have on an
// ActionActivity and neither engine serializes. Meanwhile both writers emitted a
// hardcoded literal "Rollback" on the ACTION, which is where errorHandlingType
// actually lives.
//
// WHICH VALUES MENDIX ACCEPTS, measured on 11.14.0 by writing each into a stored
// document of a 0-error project and building — the split is not guessable from
// the metamodel, where one enum covers every action type:
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
// That table is why the hardcoded "Rollback" turned out to be load-bearing rather
// than lazy, and why the fix carries the value ONLY when the author wrote a
// clause: a NANOFLOW's no-clause default is Abort (ehType), and Abort is CE6035
// on most types, so serializing the builder's value unconditionally would have
// broken every nanoflow that logs or creates.
//
// The first version of this measurement was VACUOUS and said so only because the
// probe reported its own patch count: the Delete writer emits no
// ErrorHandlingType key at all, so setting it changed nothing and mxbuild's
// silence read as "accepted". The probe was changed to INSERT an absent key.

func TestExplicitErrorHandling_NoClauseLeavesTheWriterAlone(t *testing.T) {
	fb := &flowBuilder{}
	if got := explicitErrorHandling(fb, nil); got != "" {
		t.Errorf("no ON ERROR clause produced %q — anything but empty overrides the "+
			"writer's literal, and for a nanoflow that literal would become Abort (CE6035)", got)
	}
}

func TestExplicitErrorHandling_ClauseIsCarried(t *testing.T) {
	fb := &flowBuilder{}
	got := explicitErrorHandling(fb, &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue})
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("`on error continue` produced %q, want %q", got, microflows.ErrorHandlingTypeContinue)
	}
}

// THE CONTROL for the nanoflow hazard the empty return exists to avoid. A
// nanoflow with no clause defaults to Abort, which mxbuild rejects on Log,
// Create, Change, Commit, MicroflowCall and Aggregate. If this ever returns
// Abort, un-annotated nanoflow activities start failing the build.
func TestExplicitErrorHandling_NanoflowNoClauseIsStillEmpty(t *testing.T) {
	fb := &flowBuilder{isNanoflow: true}
	if got := explicitErrorHandling(fb, nil); got != "" {
		t.Errorf("a nanoflow with no clause produced %q — that value reaches the model "+
			"now that the writers carry it, and Abort is CE6035 on most activity types", got)
	}
	// ...but an explicit clause on a nanoflow is still the author's.
	got := explicitErrorHandling(fb, &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue})
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("an explicit clause on a nanoflow produced %q", got)
	}
}

// ---------------------------------------------------------------------------
// MDL076 — Continue on an activity Mendix rejects it for
// ---------------------------------------------------------------------------

// The live bug this closes, in the other direction: CREATE is one of the few
// statements whose builder already set the value on the action, so mxcli DID
// write `Continue` there — and mxbuild refuses the result. mxcli check passed.
func TestMDL076_ReportsContinueOnCreateAndCommit(t *testing.T) {
	for _, stmt := range []ast.MicroflowStatement{
		&ast.CreateObjectStmt{ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}},
		&ast.MfCommitStmt{ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}},
	} {
		v := &microflowValidator{mfName: "M.ACT_X"}
		v.checkErrorHandlingContinueSupported(stmt)
		if len(v.violations) != 1 || v.violations[0].RuleID != continueUnsupportedRule {
			t.Errorf("%T with `on error continue` was accepted: %+v", stmt, v.violations)
			continue
		}
		if got := v.violations[0].Message; got == "" {
			t.Error("empty message")
		}
	}
}

// CONTROL: the three statements Mendix DOES accept Continue on must stay silent,
// or the fix that carries the clause would be rejected by the rule guarding it.
func TestMDL076_SilentOnTheStatementsThatAcceptContinue(t *testing.T) {
	for _, stmt := range []ast.MicroflowStatement{
		&ast.RetrieveStmt{ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}},
		&ast.DeleteObjectStmt{ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}},
		&ast.CallMicroflowStmt{ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}},
	} {
		v := &microflowValidator{mfName: "M.ACT_X"}
		v.checkErrorHandlingContinueSupported(stmt)
		if len(v.violations) != 0 {
			t.Errorf("%T was reported, but Mendix accepts Continue there: %+v", stmt, v.violations)
		}
	}
}

// CONTROL: only CONTINUE is refused. Rollback is the default and a custom
// handler was measured to build on every action type, so reporting either would
// reject working scripts — and a custom handler is the fix the rule suggests.
func TestMDL076_OnlyContinueIsRefused(t *testing.T) {
	for _, eh := range []*ast.ErrorHandlingClause{
		nil,
		{Type: ast.ErrorHandlingRollback},
		{Type: ast.ErrorHandlingCustom},
		{Type: ast.ErrorHandlingCustomWithoutRollback},
	} {
		v := &microflowValidator{mfName: "M.ACT_X"}
		v.checkErrorHandlingContinueSupported(&ast.CreateObjectStmt{ErrorHandling: eh})
		if len(v.violations) != 0 {
			t.Errorf("clause %+v was reported on a create: %+v", eh, v.violations)
		}
	}
}
