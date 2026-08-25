// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #903: a .test.mdl written with CRLF line endings panicked the parser
// with "slice bounds out of range [:-1]".
//
// The cause was a line-number estimate that searched the RAW file content for
// the first 20 characters of a block that had already been rebuilt by joining
// scanner lines with "\n" — so under CRLF the needle never matched, Index
// returned -1, and content[:-1] panicked. The block/line pairing introduced for
// #927 removed that search, which fixed this as a side effect and left it
// untested. These tests pin it: the parser must never look for reconstructed
// text inside the original bytes.
//
// CRLF is not exotic — Windows editors and text-mode file handling produce it
// silently — so every shape a test file comes in is covered here.

// writeBytes writes content verbatim, without the line-ending rewriting a
// literal source file in this repo would be subject to.
func writeBytes(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func TestParseTestFileAcceptsCRLF(t *testing.T) {
	dir := t.TempDir()
	path := writeBytes(t, dir, "crlf.test.mdl",
		"/**\r\n * @test crlf probe\r\n * @expect $x = 1\r\n */\r\n$x = 1;\r\n/\r\n")

	suite, err := ParseTestFile(path)
	if err != nil {
		t.Fatalf("ParseTestFile: %v", err)
	}
	if len(suite.Tests) != 1 {
		t.Fatalf("got %d tests, want 1", len(suite.Tests))
	}
	tc := suite.Tests[0]
	if tc.Name != "crlf probe" {
		t.Errorf("Name = %q, want %q", tc.Name, "crlf probe")
	}
	if len(tc.Expects) != 1 {
		t.Errorf("got %d @expect, want 1 — the annotation did not survive CRLF", len(tc.Expects))
	}
	if strings.Contains(tc.MDL, "\r") {
		t.Errorf("MDL body still carries CR: %q", tc.MDL)
	}
	if tc.MDL != "$x = 1;" {
		t.Errorf("MDL = %q, want %q", tc.MDL, "$x = 1;")
	}
}

// A CRLF file must produce the same tests as the identical LF file, down to the
// reported line numbers. Line numbers are where the old defect lived, so a test
// that only checked "does not panic" would not have detected a wrong answer.
func TestParseTestFileCRLFMatchesLF(t *testing.T) {
	const lf = "/**\n * @setup Fixtures.Seed\n */\n/\n" +
		"/**\n * @test first\n * @expect $a = 1\n */\n$a = 1;\n/\n" +
		"/**\n * @test second\n * @throws 'boom'\n */\n$b = 2;\n/\n"

	dir := t.TempDir()
	lfSuite, err := ParseTestFile(writeBytes(t, dir, "lf.test.mdl", lf))
	if err != nil {
		t.Fatalf("LF control failed to parse: %v", err)
	}
	crlfSuite, err := ParseTestFile(writeBytes(t, dir, "crlf.test.mdl",
		strings.ReplaceAll(lf, "\n", "\r\n")))
	if err != nil {
		t.Fatalf("CRLF: %v", err)
	}

	if len(lfSuite.Tests) != 2 {
		t.Fatalf("LF control produced %d tests, want 2 — the control itself is wrong", len(lfSuite.Tests))
	}
	if len(crlfSuite.Tests) != len(lfSuite.Tests) {
		t.Fatalf("CRLF produced %d tests, LF produced %d", len(crlfSuite.Tests), len(lfSuite.Tests))
	}
	for i := range lfSuite.Tests {
		want, got := lfSuite.Tests[i], crlfSuite.Tests[i]
		if got.Name != want.Name {
			t.Errorf("test %d: Name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Line != want.Line {
			t.Errorf("test %d (%s): Line = %d, want %d", i, want.Name, got.Line, want.Line)
		}
		if got.MDL != want.MDL {
			t.Errorf("test %d (%s): MDL = %q, want %q", i, want.Name, got.MDL, want.MDL)
		}
		if got.Throws != want.Throws {
			t.Errorf("test %d (%s): Throws = %q, want %q", i, want.Name, got.Throws, want.Throws)
		}
		if strings.Join(got.Setups, ",") != strings.Join(want.Setups, ",") {
			t.Errorf("test %d (%s): Setups = %v, want %v", i, want.Name, got.Setups, want.Setups)
		}
	}
}

// The markdown flavour reaches extractDocAndBody by a different route, so it
// needs its own coverage.
func TestParseMarkdownTestFileAcceptsCRLF(t *testing.T) {
	const lf = "# Suite\n\n```mdl-test\n/**\n * @test md crlf\n * @expect $x = 1\n */\n$x = 1;\n```\n"

	dir := t.TempDir()
	suite, err := ParseTestFile(writeBytes(t, dir, "md.test.md", strings.ReplaceAll(lf, "\n", "\r\n")))
	if err != nil {
		t.Fatalf("ParseTestFile: %v", err)
	}
	if len(suite.Tests) != 1 {
		t.Fatalf("got %d tests, want 1", len(suite.Tests))
	}
	if suite.Tests[0].Name != "md crlf" {
		t.Errorf("Name = %q, want %q", suite.Tests[0].Name, "md crlf")
	}
	if len(suite.Tests[0].Expects) != 1 {
		t.Errorf("got %d @expect, want 1", len(suite.Tests[0].Expects))
	}
}
