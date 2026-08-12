// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// upstream #825: `mxcli new` pointed `mx create-project` straight at the output
// directory. MxToolset refuses to extract past a 259-character full path and
// aborts PART WAY THROUGH when it hits that, so a deep output directory left the
// user with a few hundred orphaned files and no .mpr.
//
// Measured on mxbuild 11.13.0 by bisecting the output-directory length:
//
//	77 characters → project created, .mpr present
//	78 characters → PathTooLongException, 259 files left behind, no .mpr
//
// The blank template's longest relative path is 181 characters there, and
// 77 + 1 + 181 = 259 exactly — so the arithmetic below is the tool's real rule,
// not a guess.
func TestPathBudgetArithmeticMatchesMeasuredThreshold(t *testing.T) {
	const measuredLongest = 181 // blank 11.13.0 template
	// The last output directory length that worked, and the first that did not.
	for _, tc := range []struct {
		destLen  int
		wantWarn bool
	}{
		{77, false},
		{78, true},
	} {
		dest := "/" + strings.Repeat("x", tc.destLen-1)
		var buf bytes.Buffer
		got := warnIfPathTooLongForStudioPro(&buf, dest, measuredLongest, "some/deep/file")
		if got != tc.wantWarn {
			t.Errorf("dest of %d characters: warned=%v, want %v (77 created a project on 11.13.0, 78 did not)",
				tc.destLen, got, tc.wantWarn)
		}
		if got && !strings.Contains(buf.String(), "at most 77") {
			t.Errorf("the warning must name the budget the user can actually hit, got:\n%s", buf.String())
		}
	}
}

// The warning has to carry enough to act on: both numbers that make up the
// total, and what to do. A bare "path too long" reproduces the original problem
// — the reporter had to work out the 259 limit and the 182-character template
// path themselves.
func TestPathWarningIsActionable(t *testing.T) {
	var buf bytes.Buffer
	if !warnIfPathTooLongForStudioPro(&buf, "/"+strings.Repeat("d", 99), 182, "deep/template/path.xcscheme") {
		t.Fatal("expected a warning for a 100-character destination with a 182-character template path")
	}
	out := buf.String()
	for _, want := range []string{"100 characters", "182", "259", "deep/template/path.xcscheme", "at most 76"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning should mention %q, got:\n%s", want, out)
		}
	}
}

// longestRelativePath is measured rather than hardcoded because the template
// changes between Mendix versions (181 on 11.13.0, 182 reported on 11.12.0) — a
// constant would drift and mis-report the budget.
func TestLongestRelativePath(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "bb", "ccc", "dddd")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "leaf.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "short.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, p := longestRelativePath(root)
	want := filepath.Join("a", "bb", "ccc", "dddd", "leaf.txt")
	if p != want || n != len(want) {
		t.Errorf("longestRelativePath = (%d, %q), want (%d, %q)", n, p, len(want), want)
	}
}

// The property that matters most: a FAILED creation must leave the user's
// directory exactly as it was. Before staging, the destination held 259 orphaned
// files and no .mpr — output that looks like a project until you try to open it.
func TestStagingLeavesDestinationUntouchedOnFailure(t *testing.T) {
	stage, cleanup, err := stagedProjectDirs()
	if err != nil {
		t.Fatalf("stagedProjectDirs: %v", err)
	}
	// Simulate a partial extraction: files land in the staging directory.
	if err := os.WriteFile(filepath.Join(stage, "partial.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup() // what the deferred cleanup does when create-project fails

	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Errorf("staging directory survived cleanup: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("destination holds %d entries after a failed creation, want 0 — "+
			"the whole point of staging is that a failure leaves nothing behind", len(entries))
	}
}

// A successful creation moves the tree into place, including through the
// copy fallback that a cross-filesystem staging directory forces.
func TestMoveProject(t *testing.T) {
	stage, cleanup, err := stagedProjectDirs()
	if err != nil {
		t.Fatalf("stagedProjectDirs: %v", err)
	}
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(stage, "mprcontents", "aa"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"App.mpr", filepath.Join("mprcontents", "aa", "unit.mxunit")} {
		if err := os.WriteFile(filepath.Join(stage, f), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The caller creates the destination before staging runs, so moveProject has
	// to cope with an existing empty directory — os.Rename onto one fails.
	dest := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := moveProject(stage, dest); err != nil {
		t.Fatalf("moveProject: %v", err)
	}
	for _, f := range []string{"App.mpr", filepath.Join("mprcontents", "aa", "unit.mxunit")} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("%s did not arrive at the destination: %v", f, err)
		}
	}
}
