// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// `mxcli lint` reads Starlark rules whichever AI tool a project uses, so every
// `init --tool <x>` must give it the same rule set. Only claude and opencode
// wrote them: `init --tool cursor` installed none, and lint then ran the Go
// rules alone, silently, with exit 0 (mendixlabs/mxcli#1242, ako/mxcli#1292).
func TestInitInstallsTheBundledLintRulesForEveryTool(t *testing.T) {
	bundled := 0
	_ = fs.WalkDir(lintRulesFS, "lint-rules", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".star") {
			bundled++
		}
		return nil
	})
	if bundled == 0 {
		t.Fatal("no bundled .star rules embedded — the test would prove nothing")
	}

	tools := make([]string, 0, len(SupportedTools))
	for name := range SupportedTools {
		tools = append(tools, name)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			dir := t.TempDir()
			runInit(t, []string{tool}, dir)

			found := linter.FindLintRulesDir(dir)
			if found == "" {
				t.Fatalf("init --tool %s: lint finds no rules directory, so it would run the Go rules only", tool)
			}
			rules, failures, err := linter.LoadStarlarkRulesFromDir(found)
			if err != nil || len(failures) > 0 {
				t.Fatalf("loading %s: %v %v", found, err, failures)
			}
			if len(rules) != bundled {
				t.Errorf("init --tool %s: lint loads %d Starlark rules from %s, want the %d bundled ones",
					tool, len(rules), filepath.Base(filepath.Dir(found))+"/"+filepath.Base(found), bundled)
			}
		})
	}
}

// With no rules directory, lint and report run the built-in Go rules only. That
// is a legitimate state (a scratch copy of the model, a CI checkout without
// .claude/), but it was silent and exited 0: 210 findings from 17 rules read as
// a clean result next to 1,984 from 49 (ako/mxcli#1292). Say so.
func TestProjectLintRulesSaysWhenNoStarlarkRulesAreFound(t *testing.T) {
	bare := t.TempDir()
	var warn strings.Builder
	rules := projectLintRules(bare, &warn)
	if len(rules) != len(builtinLintRules()) {
		t.Fatalf("bare project: %d rules, want only the %d built-in ones", len(rules), len(builtinLintRules()))
	}
	if !strings.Contains(warn.String(), "no .claude/lint-rules/") {
		t.Errorf("bare project: no notice that the Starlark rules are absent; stderr was %q", warn.String())
	}

	// Control: a project with the rules says nothing about them missing.
	withRules := t.TempDir()
	runInit(t, []string{"cursor"}, withRules)
	warn.Reset()
	if rules := projectLintRules(withRules, &warn); len(rules) <= len(builtinLintRules()) {
		t.Fatalf("initialised project: %d rules, want more than the %d built-in ones", len(rules), len(builtinLintRules()))
	}
	if strings.Contains(warn.String(), "no .claude/lint-rules/") {
		t.Errorf("initialised project warned that no rules were found: %q", warn.String())
	}
}
