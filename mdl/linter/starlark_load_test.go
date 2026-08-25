// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodRule = `
def check(ctx):
    return []

rule = {
    "id": "TEST001",
    "name": "Test",
    "description": "d",
    "category": "quality",
    "severity": "warning",
}
`

// #904 reported that stale rule files are "skipped silently". They are not —
// each failure is reported. But the report went to **stdout** via fmt.Printf,
// which puts diagnostics into the same stream as `--format json`/`sarif` output,
// and the caller could not act on the failures because they were not returned.
//
// Returning them lets the command decide (print to stderr, count, exit non-zero)
// instead of the library printing into whatever stream happens to be there.
func TestLoadStarlarkRulesFromDir_ReturnsFailures(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.star", goodRule)
	write(t, dir, "broken_syntax.star", "def check(ctx):\n    this is not valid ((((\n")
	write(t, dir, "no_check.star", "x = 1\n")
	// Non-.star files are not rules and must not be reported as failures.
	write(t, dir, "README.md", "not a rule")

	rules, failures, err := LoadStarlarkRulesFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("loaded %d rules, want 1", len(rules))
	}
	if len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}

	byFile := map[string]string{}
	for _, f := range failures {
		byFile[filepath.Base(f.Path)] = f.Reason
	}
	for _, name := range []string{"broken_syntax.star", "no_check.star"} {
		reason, ok := byFile[name]
		if !ok {
			t.Errorf("no failure recorded for %s", name)
			continue
		}
		if reason == "" {
			t.Errorf("%s: empty reason — the warning must say why", name)
		}
	}
	if !strings.Contains(byFile["no_check.star"], "check()") {
		t.Errorf("no_check.star reason = %q, want it to mention the missing check()", byFile["no_check.star"])
	}
}

// Failures are ordered by filename so the warning block is stable run to run.
func TestLoadStarlarkRulesFromDir_FailuresAreOrdered(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"c_bad.star", "a_bad.star", "b_bad.star"} {
		write(t, dir, name, "x = 1\n")
	}
	_, failures, err := LoadStarlarkRulesFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a_bad.star", "b_bad.star", "c_bad.star"}
	if len(failures) != len(want) {
		t.Fatalf("got %d failures, want %d", len(failures), len(want))
	}
	for i, name := range want {
		if got := filepath.Base(failures[i].Path); got != name {
			t.Errorf("[%d] = %s, want %s", i, got, name)
		}
	}
}

// A missing directory is not an error — most projects have no custom rules.
func TestLoadStarlarkRulesFromDir_MissingDirIsNotAnError(t *testing.T) {
	rules, failures, err := LoadStarlarkRulesFromDir(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Errorf("missing dir returned error %v, want nil", err)
	}
	if len(rules) != 0 || len(failures) != 0 {
		t.Errorf("got %d rules and %d failures, want none", len(rules), len(failures))
	}
}

// An empty path must not be treated as the current directory — that would load
// whatever happens to sit in ./ and attribute it to the project.
func TestLoadStarlarkRulesFromDir_EmptyPath(t *testing.T) {
	rules, failures, err := LoadStarlarkRulesFromDir("")
	if err != nil {
		t.Errorf("got error %v, want nil", err)
	}
	if len(rules) != 0 || len(failures) != 0 {
		t.Errorf("got %d rules and %d failures, want none", len(rules), len(failures))
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
