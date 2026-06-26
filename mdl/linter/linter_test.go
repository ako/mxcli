// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"context"
	"testing"
)

// stubRule is a minimal Rule implementation for testing.
type stubRule struct {
	id string
}

func (r *stubRule) ID() string                      { return r.id }
func (r *stubRule) Name() string                    { return r.id }
func (r *stubRule) Description() string             { return "" }
func (r *stubRule) DefaultSeverity() Severity       { return SeverityWarning }
func (r *stubRule) Category() string                { return "test" }
func (r *stubRule) Check(_ *LintContext) []Violation {
	return []Violation{{RuleID: r.id, Severity: SeverityWarning, Message: "hit"}}
}

// configurableRule is a stubRule that also implements Configurable.
type configurableRule struct {
	stubRule
	configuredOptions map[string]any
}

func (r *configurableRule) Configure(options map[string]any) {
	r.configuredOptions = options
}

func TestRuleFilter_AllowlistRestrictsExecution(t *testing.T) {
	lint := New(nil)
	lint.AddRule(&stubRule{"MPR001"})
	lint.AddRule(&stubRule{"MPR002"})
	lint.AddRule(&stubRule{"SEC001"})

	// Simulate the --rules allowlist applied in cmd_lint.go.
	allowed := map[string]bool{"MPR001": true}
	for _, rule := range lint.Rules() {
		if !allowed[rule.ID()] {
			lint.ConfigureRule(rule.ID(), RuleConfig{Enabled: false})
		}
	}

	violations, err := lint.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "MPR001" {
		t.Errorf("expected MPR001, got %s", violations[0].RuleID)
	}
}

func TestRuleFilter_EmptyAllowlistRunsAll(t *testing.T) {
	lint := New(nil)
	lint.AddRule(&stubRule{"MPR001"})
	lint.AddRule(&stubRule{"MPR002"})

	// No --rules flag: no ConfigureRule calls, all rules run.
	violations, err := lint.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}
}

func TestConfigurable_OptionsDeliveredBeforeCheck(t *testing.T) {
	rule := &configurableRule{stubRule: stubRule{"MPR003"}}
	lint := New(nil)
	lint.AddRule(rule)
	lint.ConfigureRule("MPR003", RuleConfig{
		Enabled:  true,
		Severity: SeverityWarning,
		Options:  map[string]any{"max_entities": 20},
	})

	_, err := lint.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rule.configuredOptions == nil {
		t.Fatal("Configure was never called")
	}
	if rule.configuredOptions["max_entities"] != 20 {
		t.Errorf("max_entities = %v, want 20", rule.configuredOptions["max_entities"])
	}
}

func TestConfigurable_NotCalledWhenNoOptions(t *testing.T) {
	rule := &configurableRule{stubRule: stubRule{"MPR003"}}
	lint := New(nil)
	lint.AddRule(rule)
	// Config exists but Options is empty — Configure should not be called.
	lint.ConfigureRule("MPR003", RuleConfig{Enabled: true})

	if _, err := lint.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rule.configuredOptions != nil {
		t.Errorf("Configure should not be called when options are empty")
	}
}

func TestRuleFilter_MultipleRulesAllowed(t *testing.T) {
	lint := New(nil)
	lint.AddRule(&stubRule{"MPR001"})
	lint.AddRule(&stubRule{"MPR002"})
	lint.AddRule(&stubRule{"SEC001"})

	allowed := map[string]bool{"MPR001": true, "SEC001": true}
	for _, rule := range lint.Rules() {
		if !allowed[rule.ID()] {
			lint.ConfigureRule(rule.ID(), RuleConfig{Enabled: false})
		}
	}

	violations, err := lint.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}
	ids := map[string]bool{}
	for _, v := range violations {
		ids[v.RuleID] = true
	}
	if !ids["MPR001"] || !ids["SEC001"] {
		t.Errorf("expected MPR001 and SEC001, got %v", ids)
	}
}
