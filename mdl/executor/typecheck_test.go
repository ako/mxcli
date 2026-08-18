// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mprbackend "github.com/mendixlabs/mxcli/mdl/backend/mpr"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// typeCheckFixture copies the shared fixture project into a temp dir, connects an
// executor to it for writing, and seeds an enumeration plus an entity that uses
// it.
//
// A real project rather than a mock: the whole point of this path is that the
// catalog answers questions about a model on disk, and every one of the three
// defects found while building it (a missing column, an absent table, an
// expression whose source text the walk never saw) would have passed a mocked
// test.
func typeCheckFixture(t *testing.T) *Executor {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "minimal.mpr")

	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })
	t.Cleanup(func() { exec.Close() })

	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(proj)+"'")
	run(t, exec, `CREATE ENUMERATION MyFirstModule.OrderStatus (Open 'Open', Closed 'Closed');`)
	run(t, exec, `CREATE PERSISTENT ENTITY MyFirstModule.Ticket (
		Title: String(100),
		Status: Enumeration(MyFirstModule.OrderStatus)
	);`)
	return exec
}

func run(t *testing.T, exec *Executor, mdl string) {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", mdl, errs)
	}
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatalf("executing %q: %v", mdl, err)
		}
	}
}

func typeCheck(t *testing.T, exec *Executor, mdl string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return exec.TypeCheckProgram(prog)
}

// TestTypeCheckProgramCatchesEnumStringLiteral is the end-to-end proof that the
// checker now checks something. Before the CatalogReader seam had an
// implementation, this ran with a nil Catalog and every semantic rule was
// skipped — a green result that meant nothing.
func TestTypeCheckProgramCatchesEnumStringLiteral(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Bug ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'x', Status = 'Open');
END;
`)

	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].RuleID != "E001" {
		t.Errorf("rule is %q, want exprcheck's own E001 — the codes are kept, not remapped", got[0].RuleID)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity is %v, want error", got[0].Severity)
	}
	// The fix must name the enum the catalog resolved, which is the part that
	// only works because AttributeEnumQN and EnumCases have data behind them.
	if !strings.Contains(got[0].Suggestion, "MyFirstModule.OrderStatus.Open") {
		t.Errorf("suggestion is %q, want the qualified enum value", got[0].Suggestion)
	}
}

// TestTypeCheckProgramAcceptsTheCorrectedForm is the control. Without it the
// test above would pass against a checker that flagged everything.
func TestTypeCheckProgramAcceptsTheCorrectedForm(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Fixed ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'x', Status = MyFirstModule.OrderStatus.Open);
END;
`)

	if len(got) != 0 {
		t.Errorf("the corrected form was flagged: %+v", got)
	}
}

// TestTypeCheckProgramSeesChangeAsWellAsCreate pins the fix for the second
// wiring defect. The adapter's default source function reads only
// ast.SourceExpr, and the visitor attaches one to a CREATE's value but not to a
// CHANGE's — so half the enum mistakes in one microflow were invisible until the
// executor supplied a source function that can render either.
func TestTypeCheckProgramSeesChangeAsWellAsCreate(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Both ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Status = 'Open');
  CHANGE $T (Status = 'Closed');
END;
`)

	if len(got) != 2 {
		t.Fatalf("got %d violations, want one for the CREATE and one for the CHANGE: %+v", len(got), got)
	}
	var sawOpen, sawClosed bool
	for _, v := range got {
		sawOpen = sawOpen || strings.Contains(v.Suggestion, "OrderStatus.Open")
		sawClosed = sawClosed || strings.Contains(v.Suggestion, "OrderStatus.Closed")
	}
	if !sawOpen || !sawClosed {
		t.Errorf("expected both values reported, got %+v", got)
	}
}

// TestTypeCheckProgramLeavesNonEnumAttributesAlone pins that the rule keys off
// the attribute's actual type, not off any string literal in a member slot.
func TestTypeCheckProgramLeavesNonEnumAttributesAlone(t *testing.T) {
	exec := typeCheckFixture(t)

	got := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Strings ()
BEGIN
  $T = CREATE MyFirstModule.Ticket (Title = 'Open');
END;
`)

	if len(got) != 0 {
		t.Errorf("a String attribute assigned a string literal was flagged: %+v", got)
	}
}

// TestTypeCheckProgramWithoutAConnectionIsSilent pins the advisory contract: a
// caller that cannot consult a project gets no violations rather than an error.
func TestTypeCheckProgramWithoutAConnectionIsSilent(t *testing.T) {
	exec := New(&bytes.Buffer{})
	defer exec.Close()

	prog, errs := visitor.Build(`CREATE MICROFLOW M.A () BEGIN LOG 'x'; END;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if got := exec.TypeCheckProgram(prog); got != nil {
		t.Errorf("an unconnected executor reported %+v", got)
	}
	if got := exec.TypeCheckProgram(nil); got != nil {
		t.Errorf("a nil program reported %+v", got)
	}
}
