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
