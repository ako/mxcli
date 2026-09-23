// SPDX-License-Identifier: Apache-2.0

package executor

// `mxcli exec` ran only ONE of the two validation passes `mxcli check -p` runs,
// so a name that resolves to nothing reached the model unreported (#607).
//
// MEASURED on this fixture with the pre-fix binary (`--no-check` reproduces it),
// because the failure mode is NOT the "half-applied model" exec's other refusal
// describes:
//
//	create entity "NotAModule"."Thing"      exit 0 — "Created module: NotAModule"
//	microflow retrieving a missing entity   exit 0 — both documents written
//
// exec completed in both cases. It silently created a module on a misspelling,
// and wrote a dangling entity reference that only mxbuild would reject (CE1613).
//
// The control is the FIRST assertion below: the pass exec ran reports nothing at
// all for a script the reference pass refuses. Without it this test would pass
// against a build where exec had never been missing anything.

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// connectedExecutor returns an executor connected to the shared fixture, which
// is what makes the reference pass answerable at all.
func connectedExecutor(t *testing.T, projectPath string) *Executor {
	t.Helper()
	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectPath)+"'")
	return exec
}

// A reference to a module that does not exist is the simplest dangling
// reference there is: it needs the project to detect and nothing else.
const danglingReferenceScript = `create entity "NotAModule"."Thing" ( "Name": String(50) );`

func TestReferenceErrorsAreInvisibleToTheSemanticPass(t *testing.T) {
	p := projectFixture(t)

	prog, errs := visitor.Build(danglingReferenceScript)
	if len(errs) > 0 {
		t.Fatalf("fixture script does not parse: %v", errs)
	}

	// THE CONTROL. This is the pass `exec` runs in its preflight. It is given
	// the project path and still cannot see a missing module, because resolving
	// one needs a connected backend rather than a path.
	//
	// If this ever starts reporting an error, the gap has closed by another
	// route and the assertion below stops proving anything — so it fails loudly
	// rather than being quietly relaxed.
	if summary := linter.Summarize(ValidateProgram(prog, p)); summary.Errors > 0 {
		t.Fatalf("the semantic pass now reports %d error(s) for a dangling reference; "+
			"this test's control has expired and the preflight gap must be re-established "+
			"before the assertion below means anything", summary.Errors)
	}

	// The pass `check -p` runs, and `exec` does not.
	refErrs := connectedExecutor(t, p).ValidateProgram(prog)
	if len(refErrs) == 0 {
		t.Fatal("the reference pass reports nothing for a script naming a module that does " +
			"not exist — then `check -p` is not catching it either, and #607 is not the bug")
	}
}
