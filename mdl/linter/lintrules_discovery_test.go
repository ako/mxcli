// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"testing"
)

// #904: rule discovery was `filepath.Dir(<the .mpr>) + /.claude/lint-rules`, so
// the bundled rules were found only when `.claude/` sat in the same directory as
// the .mpr. In the layout `workspace/.claude/lint-rules/` + `workspace/app/x.mpr`
// — which is both what `mxcli init` used to create and the ordinary Claude Code
// convention of one `.claude/` at the repo root — `mxcli lint` loaded **zero**
// Starlark rules and said nothing. Measured: 19 rules found vs 48 with the same
// files one directory down.
func TestFindLintRulesDir(t *testing.T) {
	// repo/
	//   .git/
	//   .claude/lint-rules/        <- ancestor copy
	//   app/
	//     .claude/lint-rules/      <- nearest copy
	//     nested/
	repo := t.TempDir()
	mustMkdir(t, filepath.Join(repo, ".git"))
	rootRules := filepath.Join(repo, ".claude", "lint-rules")
	mustMkdir(t, rootRules)
	appRules := filepath.Join(repo, "app", ".claude", "lint-rules")
	mustMkdir(t, appRules)
	nested := filepath.Join(repo, "app", "nested")
	mustMkdir(t, nested)

	t.Run("finds the directory beside the project", func(t *testing.T) {
		got := FindLintRulesDir(filepath.Join(repo, "app"))
		if got != appRules {
			t.Errorf("got %q, want %q", got, appRules)
		}
	})

	t.Run("finds an ancestor directory — the #904 layout", func(t *testing.T) {
		// A project directory with no .claude/ of its own must still find the
		// one at the repo root, instead of silently loading no rules.
		got := FindLintRulesDir(nested)
		if got != appRules {
			t.Errorf("got %q, want %q (nearest ancestor wins)", got, appRules)
		}
	})

	t.Run("nearest wins over the repo root", func(t *testing.T) {
		// app/ has its own copy; the root copy must not shadow it.
		if got := FindLintRulesDir(filepath.Join(repo, "app")); got == rootRules {
			t.Errorf("got the root copy %q, want the nearer %q", got, appRules)
		}
	})

	t.Run("reaches the repo root when nothing is nearer", func(t *testing.T) {
		bare := filepath.Join(repo, "other", "deep")
		mustMkdir(t, bare)
		if got := FindLintRulesDir(bare); got != rootRules {
			t.Errorf("got %q, want %q", got, rootRules)
		}
	})
}

// The search must not escape the repository. Walking to the filesystem root
// would let an unrelated `.claude/` in a home directory or /tmp silently supply
// lint rules to every project underneath it — a worse failure than finding none,
// because the rules would be someone else's.
func TestFindLintRulesDir_StopsAtRepoRoot(t *testing.T) {
	outer := t.TempDir()
	strayRules := filepath.Join(outer, ".claude", "lint-rules")
	mustMkdir(t, strayRules)

	repo := filepath.Join(outer, "repo")
	mustMkdir(t, filepath.Join(repo, ".git"))
	project := filepath.Join(repo, "app")
	mustMkdir(t, project)

	if got := FindLintRulesDir(project); got != "" {
		t.Errorf("got %q, want \"\" — the search crossed the repo root into an unrelated .claude/", got)
	}
}

func TestFindLintRulesDir_NoneAnywhere(t *testing.T) {
	repo := t.TempDir()
	mustMkdir(t, filepath.Join(repo, ".git"))
	project := filepath.Join(repo, "app")
	mustMkdir(t, project)

	if got := FindLintRulesDir(project); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A file named .claude/lint-rules is not a rule directory.
func TestFindLintRulesDir_IgnoresNonDirectory(t *testing.T) {
	repo := t.TempDir()
	mustMkdir(t, filepath.Join(repo, ".git"))
	mustMkdir(t, filepath.Join(repo, ".claude"))
	if err := os.WriteFile(filepath.Join(repo, ".claude", "lint-rules"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := FindLintRulesDir(repo); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFindLintRulesDir_EmptyInput(t *testing.T) {
	if got := FindLintRulesDir(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}
