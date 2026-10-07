// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RENAME JAVA ACTION moved javasource/<module>/actions/<Old>.java to <New>.java
// and left `public class Old`, its constructor and toString's "Old" inside, so
// the project's own source was not valid Java: javac requires a public class to
// live in a file of its name. A full build hides it — mxbuild regenerates the
// stub in its copy — but `run --local --watch` hot reload compiles the file on
// disk, and so does every IDE.
//
// The goldens are measured, not written: JA_Renamed.before is what mxcli left
// after `mxcli rename java-action R1318.JA_Old JA_New` on a fresh 11.14.0 app,
// hand-edited to add an import, extra code, and user code that mentions JA_Old
// in a string and in comments; JA_Renamed is the same file after mxbuild ran on
// the project in place. mxbuild changed exactly three lines — class, constructor,
// toString — and kept the user code, extra code and CRLF as they were.
func TestRenameSourceMatchesMxbuild(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("testdata", "mxbuild", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	before := read("JA_Renamed.before.java.golden")
	want := read("JA_Renamed.java.golden")

	got := RenameSource(before, "JA_Old", "JA_New")
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

// A name that is a prefix of another identifier, or of the new name, must not
// be rewritten inside it.
func TestRenameSourceWholeWordsOnly(t *testing.T) {
	src := "public class JA extends UserAction<JA_Other>\n{\n\tpublic JA(\n\t)\n\t{\n\t}\n\tpublic java.lang.String toString()\n\t{\n\t\treturn \"JA\";\n\t}\n}\n"
	want := "public class JA_X extends UserAction<JA_Other>\n{\n\tpublic JA_X(\n\t)\n\t{\n\t}\n\tpublic java.lang.String toString()\n\t{\n\t\treturn \"JA_X\";\n\t}\n}\n"
	if got := RenameSource(src, "JA", "JA_X"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
