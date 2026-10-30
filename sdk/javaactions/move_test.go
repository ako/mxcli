// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// MOVE JAVA ACTION A.X TO B left javasource/a/actions/X.java behind. mxbuild
// generates the stub where the model now puts the action, and that is the class
// the app calls: measured on 11.13.0, the build succeeded, jamb/actions/JA_Hello
// threw "Java action was not implemented", and the author's code compiled only as
// an orphaned jama.actions class nothing calls.
//
// The goldens are measured, not written: JA_Moved.before is the source moved to
// javasource/jamb/actions/ with nothing else changed — hand-edited first to add
// an import, extra code, and user code that mentions jama.actions in a comment
// and a string — and JA_Moved is the same file after mxbuild 11.13.0 ran on the
// project in place. mxbuild changed exactly one line, the package declaration,
// and kept everything the author wrote, CRLF line endings included.
func TestMoveSourceMatchesMxbuild(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("testdata", "mxbuild", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	before := read("JA_Moved.before.java.golden")
	want := read("JA_Moved.java.golden")

	got := MoveSource(before, "JamA", "JamB")
	if got != want {
		gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("line %d differs from mxbuild's regeneration:\n got: %q\nwant: %q", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("length differs from mxbuild's regeneration: %d lines, want %d", len(gl), len(wl))
	}
}

// A move within one module directory changes nothing, and the package match is
// the whole module name: moving out of "Sales" must not touch "salesextra".
func TestMoveSourceRewritesOnlyThePackageItNames(t *testing.T) {
	src := "package salesextra.actions;\n\npublic class X {}\n"
	if got := MoveSource(src, "Sales", "Billing"); got != src {
		t.Errorf("rewrote another module's package:\n%s", got)
	}
	same := "package sales.actions;\n"
	if got := MoveSource(same, "Sales", "SALES"); got != same {
		t.Errorf("a move to the same lowercased directory rewrote the source: %q", got)
	}
}
