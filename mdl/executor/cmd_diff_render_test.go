// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #997: diff rendered its script side with a second AST-to-MDL renderer whose
// statement switch had no default case, so an activity it did not know emitted
// zero lines and showed up in the diff as a deletion. mxcli reported that a
// script would gut a microflow that exec proved was a no-op.
//
// The fix is structural — one renderer for both sides — so the regression test
// that matters is that the dead renderer stays dead. A reviewer adding a case
// to a revived script-side renderer would re-create the drift; this fails
// first and says why.
func TestDiffHasNoSecondFlowRenderer(t *testing.T) {
	for _, name := range []string{
		"microflowStmtToMDL",
		"nanoflowStmtToMDL",
		"microflowStatementToMDL",
		"diffExpressionToString",
	} {
		if diffMDLSource(t, name) {
			t.Errorf("%s is back in cmd_diff_mdl.go. diff must render its script side "+
				"through renderMicroflowMDL — the describer both `describe` and `diff-local` "+
				"use — so that an activity it cannot print is impossible rather than shown "+
				"as a deletion (#997).", name)
		}
	}
}

// diffMDLSource reports whether cmd_diff_mdl.go still defines fn.
func diffMDLSource(t *testing.T, fn string) bool {
	t.Helper()
	b, err := os.ReadFile("cmd_diff_mdl.go")
	if err != nil {
		t.Fatalf("read cmd_diff_mdl.go: %v", err)
	}
	return strings.Contains(string(b), "func "+fn+"(")
}

// nanoflowAsMicroflow must carry ContainerID: renderMicroflowMDL prints the
// `folder` line from it, so dropping it would make one side of a nanoflow diff
// print a folder and the other not — a false modification of exactly the kind
// this issue was about.
func TestNanoflowWrapperCarriesContainer(t *testing.T) {
	nf := &microflows.Nanoflow{Name: "NF", ContainerID: "folder-id"}
	got := nanoflowAsMicroflow(nf)
	if got.ContainerID != "folder-id" {
		t.Errorf("ContainerID = %q, want folder-id — the folder line would differ between sides", got.ContainerID)
	}
	if nanoflowAsMicroflow(nil) != nil {
		t.Error("nil nanoflow should wrap to nil")
	}
}

// A statement diff cannot compare must be reported, not dropped. Silently
// skipping made `diff` print "0 new, 0 modified, 0 unchanged" for a script that
// would genuinely add documents — worse than a wrong count, because there is
// nothing on screen to disbelieve.
func TestUnsupportedStatementIsAnError(t *testing.T) {
	_, err := diffStatement(nil, &ast.CreateConstantStmt{})
	if err == nil {
		t.Fatal("an unsupported statement returned no error — it would vanish from the summary")
	}
	var unsupported *unsupportedDiffError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %T, want *unsupportedDiffError", err)
	}
	if !strings.Contains(unsupported.kind, "constant") {
		t.Errorf("kind = %q, want it to name the statement", unsupported.kind)
	}
}

func TestStatementKindName(t *testing.T) {
	if got := statementKindName(&ast.CreateConstantStmt{}); got != "create constant" {
		t.Errorf("got %q, want \"create constant\"", got)
	}
}
