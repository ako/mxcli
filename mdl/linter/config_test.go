// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "lint-config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write YAML: %v", err)
	}
	return path
}

func TestLoadConfig_ExcludedModules(t *testing.T) {
	path := writeYAML(t, `
excludeModules:
  - Administration
  - Anonymous
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.ExcludeModules) != 2 {
		t.Fatalf("want 2 excluded modules, got %d", len(cfg.ExcludeModules))
	}
	if cfg.ExcludeModules[0] != "Administration" || cfg.ExcludeModules[1] != "Anonymous" {
		t.Errorf("unexpected modules: %v", cfg.ExcludeModules)
	}
}

func TestLoadConfig_RuleEnabled(t *testing.T) {
	path := writeYAML(t, `
rules:
  MPR001:
    enabled: false
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	rule, ok := cfg.Rules["MPR001"]
	if !ok {
		t.Fatal("MPR001 rule not found")
	}
	if rule.Enabled == nil || *rule.Enabled != false {
		t.Errorf("expected enabled=false, got %v", rule.Enabled)
	}
}

func TestLoadConfig_RuleSeverity(t *testing.T) {
	path := writeYAML(t, `
rules:
  PH009:
    severity: hint
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Rules["PH009"].Severity != "hint" {
		t.Errorf("expected severity hint, got %q", cfg.Rules["PH009"].Severity)
	}
}

func TestLoadConfig_RuleOptions(t *testing.T) {
	path := writeYAML(t, `
rules:
  MPR003:
    options:
      max_entities: 20
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	opts := cfg.Rules["MPR003"].Options
	if opts == nil {
		t.Fatal("expected options map, got nil")
	}
	// YAML numbers unmarshal as int when they fit.
	v, ok := opts["max_entities"]
	if !ok {
		t.Fatal("max_entities not found in options")
	}
	switch n := v.(type) {
	case int:
		if n != 20 {
			t.Errorf("max_entities = %d, want 20", n)
		}
	case float64:
		if n != 20 {
			t.Errorf("max_entities = %v, want 20", n)
		}
	default:
		t.Errorf("unexpected type %T for max_entities", v)
	}
}

func TestLoadConfig_MissingFileReturnsDefault(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/lint-config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(cfg.ExcludeModules) != 0 || len(cfg.Rules) != 0 {
		t.Errorf("expected empty default config, got %+v", cfg)
	}
}

func TestApplyConfig_DisablesRule(t *testing.T) {
	path := writeYAML(t, `
rules:
  MPR001:
    enabled: false
`)
	cfg, _ := LoadConfig(path)

	lint := New(nil)
	lint.AddRule(&stubRule{"MPR001"})
	lint.AddRule(&stubRule{"MPR002"})
	cfg.ApplyConfig(lint)

	configs := lint.configs
	if c, ok := configs["MPR001"]; !ok || c.Enabled {
		t.Errorf("MPR001 should be disabled, got %+v", c)
	}
	if c, ok := configs["MPR002"]; ok && !c.Enabled {
		t.Errorf("MPR002 should not be disabled, got %+v", c)
	}
}

func TestApplyConfig_OverridesSeverity(t *testing.T) {
	path := writeYAML(t, `
rules:
  MPR001:
    severity: error
`)
	cfg, _ := LoadConfig(path)

	lint := New(nil)
	lint.AddRule(&stubRule{"MPR001"})
	cfg.ApplyConfig(lint)

	if c := lint.configs["MPR001"]; c.Severity != SeverityError {
		t.Errorf("expected SeverityError, got %v", c.Severity)
	}
}

func TestApplyConfig_PassesOptions(t *testing.T) {
	path := writeYAML(t, `
rules:
  MPR003:
    options:
      max_entities: 25
`)
	cfg, _ := LoadConfig(path)

	lint := New(nil)
	lint.AddRule(&stubRule{"MPR003"})
	cfg.ApplyConfig(lint)

	opts := lint.configs["MPR003"].Options
	if opts == nil {
		t.Fatal("expected options in RuleConfig, got nil")
	}
}

func TestFindConfigFile_LocatesRootFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lint-config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := FindConfigFile(dir)
	if got != path {
		t.Errorf("FindConfigFile = %q, want %q", got, path)
	}
}

func TestFindConfigFile_LocatesDotClaudeFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sub, "lint-config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := FindConfigFile(dir)
	if got != path {
		t.Errorf("FindConfigFile = %q, want %q", got, path)
	}
}

func TestFindConfigFile_ReturnsEmptyWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if got := FindConfigFile(dir); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
