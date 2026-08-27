// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §5: `mxcli test <dir>` reported "no tests found" for a
// directory holding workflow.mdl, with no hint that the NAME is why — and the
// line above it said "0 test(s) in 1 file(s)", where the 1 was the directory.
package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dirWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("show entities;\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return dir
}

func TestEmptySuiteNamesTheSkippedCandidates(t *testing.T) {
	// A .mdl file in a test directory is almost always the intended test under the
	// wrong name, so naming it turns a dead end into a rename.
	dir := dirWith(t, "workflow.mdl", "notes.md", "readme.txt")
	msg := emptySuiteError([]string{dir}).Error()

	if !strings.Contains(msg, "workflow.mdl") || !strings.Contains(msg, "notes.md") {
		t.Errorf("skipped candidates not named:\n%s", msg)
	}
	if !strings.Contains(msg, ".test.mdl") {
		t.Errorf("message does not say what a test file is named:\n%s", msg)
	}
	// A .txt was never a candidate; listing it would be noise.
	if strings.Contains(msg, "readme.txt") {
		t.Errorf("unrelated file listed as a candidate:\n%s", msg)
	}
}

func TestEmptySuiteOnATrulyEmptyDirectoryDoesNotInvent(t *testing.T) {
	// The control. With nothing to point at, the message must not claim files were
	// skipped — it explains the format instead.
	msg := emptySuiteError([]string{dirWith(t)}).Error()
	if strings.Contains(msg, "were not read") {
		t.Errorf("claimed files were skipped in an empty directory:\n%s", msg)
	}
	if !strings.Contains(msg, "@test") {
		t.Errorf("message does not describe a test block:\n%s", msg)
	}
}

func TestFilesReadCountsFilesNotPaths(t *testing.T) {
	// "0 test(s) in 1 file(s)" for a directory nothing was read from is what made
	// the old message read as though a file had been opened and found wanting. One
	// path in, zero files read.
	dir := dirWith(t, "workflow.mdl")
	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.FilesRead != 0 {
		t.Errorf("FilesRead = %d for a directory with no test-named file, want 0", suite.FilesRead)
	}

	// And the positive control, so this is not passing because the counter is
	// simply never incremented.
	suite, err = ParseTestDir(dirWith(t, "workflow.mdl", "smoke.test.mdl"))
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.FilesRead != 1 {
		t.Errorf("FilesRead = %d with one test-named file, want 1", suite.FilesRead)
	}
}
