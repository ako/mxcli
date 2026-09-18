// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#525: `mxcli report` never loaded lint-config.yaml, so a rule the
// team had deliberately accepted and disabled still scored against the project.
// On the reporting project 61 of 86 findings were two such rules, and the score
// (66/100 against a 99/100 blank-app baseline) could not be moved by any
// configuration — which reads as the tool disagreeing with a decision rather
// than as the tool not having read the file.
//
// The sibling defect in the same command: report carried its own inline copy of
// the built-in rule list, one rule behind lint's. Two commands scoring one
// project against two rule sets is the #904 class again.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// writeLintConfig drops a lint-config.yaml disabling CONV010 into a temp
// project directory, in the first location FindConfigFile searches.
func writeLintConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	claude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const cfg = `excludeModules:
  - System
rules:
  CONV010:
    enabled: false
`
	if err := os.WriteFile(filepath.Join(claude, "lint-config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

// TestApplyLintConfig_DisablesARule is the mechanism both commands depend on.
func TestApplyLintConfig_DisablesARule(t *testing.T) {
	dir := writeLintConfig(t)

	lint := linter.New(nil)
	cfg, path := applyLintConfig(lint, dir, io.Discard)
	if cfg == nil {
		t.Fatalf("no config loaded from %s", dir)
	}
	if path == "" {
		t.Error("config path is empty, so nothing would be named in a warning")
	}
	if lint.RuleEnabled("CONV010") {
		t.Error("CONV010 is still enabled after a config disabling it")
	}
	// Control: a rule the config says nothing about must stay enabled.
	// Without it, a broken ApplyConfig that disables everything passes.
	if !lint.RuleEnabled("CONV007") {
		t.Error("CONV007 was disabled by a config that never mentions it")
	}
	if len(cfg.ExcludeModules) != 1 || cfg.ExcludeModules[0] != "System" {
		t.Errorf("excludeModules = %v, want [System]", cfg.ExcludeModules)
	}
}

// TestLintAndReportShareOneRuleSet guards the inline copy, structurally.
//
// A value test cannot catch this one: both commands build their rules inside a
// cobra RunE, so nothing a unit test can call would notice a second list being
// re-added beside the shared helper — which is exactly how report's copy drifted
// a rule behind in the first place. So this asserts the property that matters:
// neither command constructs rules itself. The precedent is
// scripts/check-tunnel-deps.sh, which guards an import graph the same way.
func TestLintAndReportShareOneRuleSet(t *testing.T) {
	// Positive control first, so the test cannot pass by finding nothing:
	// builtinLintRules is the ONE place allowed to construct rules, and it must
	// still be doing so.
	setup, err := os.ReadFile("cmd_lint.go")
	if err != nil {
		t.Fatalf("read cmd_lint.go: %v", err)
	}
	if !strings.Contains(string(setup), "rules.New") {
		t.Fatal("builtinLintRules no longer constructs rules — this guard would pass vacuously")
	}

	for _, name := range []string{"cmd_report.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "lint.AddRule(rules.New") {
				t.Errorf("%s:%d constructs a lint rule inline:\n\t%s\n"+
					"Rules come from projectLintRules, or the two commands score "+
					"a project against different rule sets (ako/mxcli#525).",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}

	// And the shared set really is the built-in list plus nothing, given a
	// project directory with no .claude/lint-rules/ in it. MDL-FLOW01 is the
	// rule report's inline copy had fallen behind on.
	shared := map[string]bool{}
	for _, r := range projectLintRules(t.TempDir(), io.Discard) {
		shared[r.ID()] = true
	}
	if len(shared) == 0 {
		t.Fatal("no built-in rules at all")
	}
	if !shared["MDL-FLOW01"] {
		t.Error("MDL-FLOW01 missing — the shared rule set is not the built-in list")
	}
	if len(builtinLintRules()) != len(shared) {
		t.Errorf("projectLintRules registered %d rules where the built-ins are %d, "+
			"with no rules dir present", len(shared), len(builtinLintRules()))
	}
}
