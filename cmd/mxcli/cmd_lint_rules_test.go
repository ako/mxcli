// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// newTestLinter builds a linter carrying the rule set `mxcli lint` ships, so
// these tests exercise the IDs users actually type rather than a fixture that
// can drift away from them.
func newTestLinter() *linter.Linter {
	lint := linter.New(nil)
	for _, rule := range builtinLintRules() {
		lint.AddRule(rule)
	}
	return lint
}

func TestApplyRuleAllowlistNarrowsToNamedRules(t *testing.T) {
	lint := newTestLinter()

	unknown := applyRuleAllowlist(lint, []string{"MDL-FLOW01"})
	if len(unknown) != 0 {
		t.Fatalf("MDL-FLOW01 is a shipped rule, got unknown=%v", unknown)
	}

	enabled := lint.EnabledRuleIDs()
	if len(enabled) != 1 || enabled[0] != "MDL-FLOW01" {
		t.Errorf("want only MDL-FLOW01 enabled, got %v", enabled)
	}
}

// The regression control. An ID matching no rule used to fall through the
// allowlist and disable *every* rule, so the run reported zero findings and
// read exactly like a clean project. Stub the refusal in applyRuleAllowlist
// and this fails with an empty enabled set — the reported symptom.
func TestApplyRuleAllowlistUnknownIDDisablesNothing(t *testing.T) {
	lint := newTestLinter()
	before := len(lint.EnabledRuleIDs())

	unknown := applyRuleAllowlist(lint, []string{"MDL-FLOW99"})
	if len(unknown) != 1 || unknown[0] != "MDL-FLOW99" {
		t.Fatalf("want MDL-FLOW99 reported unknown, got %v", unknown)
	}

	if after := len(lint.EnabledRuleIDs()); after != before {
		t.Errorf("a refused allowlist must not change the rule set: %d rules enabled before, %d after", before, after)
	}
}

func TestApplyRuleAllowlistReportsEveryUnknownID(t *testing.T) {
	lint := newTestLinter()

	// A known ID alongside unknown ones must not mask them: reporting only
	// the first would send the user round the loop once per typo.
	unknown := applyRuleAllowlist(lint, []string{"MPR001", "NOPE1", "MPR001", "NOPE2", "NOPE1"})
	want := []string{"NOPE1", "NOPE2"}
	if len(unknown) != len(want) {
		t.Fatalf("want %v, got %v", want, unknown)
	}
	for i := range want {
		if unknown[i] != want[i] {
			t.Errorf("unknown[%d] = %q, want %q", i, unknown[i], want[i])
		}
	}
}

// The allowlist only disables, so a rule the config file already turned off
// stays off — naming exactly those rules leaves nothing to run. The command
// refuses on this rather than reporting a clean project; pinned here because
// the emptiness, not the refusal, is the part that is easy to reintroduce.
func TestApplyRuleAllowlistCannotReEnableAConfigDisabledRule(t *testing.T) {
	lint := newTestLinter()
	lint.ConfigureRule("MDL-FLOW01", linter.RuleConfig{Enabled: false})

	if unknown := applyRuleAllowlist(lint, []string{"MDL-FLOW01"}); len(unknown) != 0 {
		t.Fatalf("MDL-FLOW01 is registered, just disabled; got unknown=%v", unknown)
	}
	if enabled := lint.EnabledRuleIDs(); len(enabled) != 0 {
		t.Errorf("want nothing enabled, got %v", enabled)
	}
}

func TestSuggestRuleID(t *testing.T) {
	rules := builtinLintRules()
	tests := []struct {
		id   string
		want string
	}{
		{"mdl-flow01", "MDL-FLOW01"}, // wrong case — what a shell history entry looks like
		{"MDLFLOW01", "MDL-FLOW01"},  // separator dropped
		{"mdl_flow01", "MDL-FLOW01"}, // separator guessed wrong
		{"mpr001", "MPR001"},
		{"ZZZ999", ""}, // nothing close enough to suggest
	}
	for _, tt := range tests {
		if got := suggestRuleID(tt.id, rules); got != tt.want {
			t.Errorf("suggestRuleID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestBuiltinLintRuleIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, rule := range builtinLintRules() {
		if seen[rule.ID()] {
			t.Errorf("duplicate rule ID %q — --rules and config overrides address rules by ID", rule.ID())
		}
		seen[rule.ID()] = true
	}
}
