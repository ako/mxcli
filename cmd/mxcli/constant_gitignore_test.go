// SPDX-License-Identifier: Apache-2.0

// The machine store's promise — "never committed, never shared" — rests
// entirely on .mxcli/ being git-ignored. `mxcli init` writes a .gitignore only
// when the project has none, and a Mendix project usually already has one, so
// the entry could simply be absent. These tests cover making it true and then
// checking it, because a store that leaks is worse than no store at all.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitProject(t *testing.T, gitignore string) string {
	t.Helper()
	dir := t.TempDir()
	if gitignore != "" {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v (%s)", err, out)
		}
	}
	return filepath.Join(dir, "App.mpr")
}

func readIgnore(t *testing.T, projectPath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(filepath.Dir(projectPath), ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	return string(body)
}

func TestEnsureStoreIgnored_AppendsToAnExistingGitignore(t *testing.T) {
	p := gitProject(t, "deployment/\n*.mpr.bak\n")

	got, err := ensureStoreIgnored(p)
	if err != nil {
		t.Fatalf("ensureStoreIgnored: %v", err)
	}
	if got != ignoreConfirmed {
		t.Errorf("status = %v, want confirmed", got)
	}
	body := readIgnore(t, p)
	if !strings.Contains(body, ".mxcli/") {
		t.Errorf(".mxcli/ was not added:\n%s", body)
	}
	// The lines that were already there stay there.
	if !strings.Contains(body, "deployment/") || !strings.Contains(body, "*.mpr.bak") {
		t.Errorf("existing entries were lost:\n%s", body)
	}
}

func TestEnsureStoreIgnored_CreatesAGitignoreWhenThereIsNone(t *testing.T) {
	p := gitProject(t, "")

	if got, err := ensureStoreIgnored(p); err != nil || got != ignoreConfirmed {
		t.Fatalf("got %v / %v, want confirmed", got, err)
	}
	if !strings.Contains(readIgnore(t, p), ".mxcli/") {
		t.Error(".mxcli/ missing from the created .gitignore")
	}
}

// Running twice must not append the block twice.
func TestEnsureStoreIgnored_IsIdempotent(t *testing.T) {
	p := gitProject(t, "deployment/\n")

	for range 3 {
		if _, err := ensureStoreIgnored(p); err != nil {
			t.Fatalf("ensureStoreIgnored: %v", err)
		}
	}
	if n := strings.Count(readIgnore(t, p), ".mxcli/"); n != 1 {
		t.Errorf(".mxcli/ appears %d times, want 1", n)
	}
}

// The check that matters: adding the line is not proof, so git is asked.
//
// Which rule defeats it is not obvious and was measured rather than assumed —
// `!.mxcli/**` does NOT re-include anything, because git cannot re-include a
// file whose parent directory is excluded. `!.mxcli` unexcludes the directory
// itself, and then the file inside is not ignored. That is the case a caller
// writing a secret has to refuse on.
func TestEnsureStoreIgnored_ReportsNotIgnoredWhenTheDirectoryIsReIncluded(t *testing.T) {
	p := gitProject(t, ".mxcli/\n!.mxcli\n")

	got, err := ensureStoreIgnored(p)
	if err != nil {
		t.Fatalf("ensureStoreIgnored: %v", err)
	}
	if got != ignoreNotIgnored {
		t.Fatalf("status = %v, want notIgnored — a negation rule defeats the entry, and "+
			"writing a secret there would leak it", got)
	}
}

// Outside a repository there is nothing to leak into, so this is "cannot tell",
// not "unsafe" — the caller proceeds and says so.
func TestEnsureStoreIgnored_UnverifiedOutsideARepository(t *testing.T) {
	p := filepath.Join(t.TempDir(), "App.mpr")

	got, err := ensureStoreIgnored(p)
	if err != nil {
		t.Fatalf("ensureStoreIgnored: %v", err)
	}
	if got != ignoreUnverified {
		t.Errorf("status = %v, want unverified outside a git repository", got)
	}
}

func TestMentionsMxcliDir(t *testing.T) {
	for _, yes := range []string{".mxcli/", ".mxcli", "/.mxcli/", "  .mxcli/  "} {
		if !mentionsMxcliDir("a\n" + yes + "\nb") {
			t.Errorf("mentionsMxcliDir(%q) = false", yes)
		}
	}
	for _, no := range []string{".mxclifoo/", "x.mxcli/", "# .mxcli/"} {
		if mentionsMxcliDir(no) {
			t.Errorf("mentionsMxcliDir(%q) = true", no)
		}
	}
}
