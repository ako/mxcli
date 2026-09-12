// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestShowMessageBlocking — `blocking` is Studio Pro's checkbox on a message
// action, and MDL had no word for it.
//
// The model carried Blocking on BOTH engines all along, so nothing in the
// storage layer was wrong; DESCRIBE simply could not emit it, and the re-parse
// set false. That is why the loss survived: every layer except the text was
// correct. Measured on 16 microflows across 4 projects.
func TestShowMessageBlocking(t *testing.T) {
	blocking := firstStatement(t, "show message 'Saved.' type Information blocking;").(*ast.ShowMessageStmt)
	if !blocking.Blocking {
		t.Error("`blocking` not parsed")
	}

	plain := firstStatement(t, "show message 'Saved.' type Information;").(*ast.ShowMessageStmt)
	if plain.Blocking {
		t.Error("Blocking set on a statement that did not say it")
	}

	// It sits after the OBJECTS clause and before ON ERROR, so all three
	// combine. (Rollback, not Continue: Mendix rejects Continue error handling
	// on a message action, which MDL076 refuses before the write.)
	full := firstStatement(t,
		"show message 'Hi {1}' type Warning objects [$Name] blocking on error rollback;").(*ast.ShowMessageStmt)
	if !full.Blocking || len(full.TemplateArgs) != 1 || full.ErrorHandling == nil {
		t.Errorf("combined clauses = %+v", full)
	}
}

// TestBlockingIsStillUsableAsAnIdentifier — adding a lexer token risks turning
// an ordinary name into a keyword. BLOCKING is listed in the `keyword` rule, so
// a parameter may still be called "blocking".
func TestBlockingIsStillUsableAsAnIdentifier(t *testing.T) {
	prog, errs := Build("create microflow M.F (blocking: Boolean)\nbegin\n  return;\nend;")
	if len(errs) > 0 {
		t.Fatalf("a parameter named `blocking` no longer parses: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if len(mf.Parameters) != 1 || mf.Parameters[0].Name != "blocking" {
		t.Errorf("parameters = %+v", mf.Parameters)
	}
}
