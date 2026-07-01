// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"testing"
)

// fakeRule implements Rule; fakeReqRule adds the optional CatalogRequirer.
type fakeRule struct {
	id string
}

func (r fakeRule) ID() string                     { return r.id }
func (r fakeRule) Name() string                   { return r.id }
func (r fakeRule) Description() string            { return "" }
func (r fakeRule) DefaultSeverity() Severity      { return SeverityWarning }
func (r fakeRule) Category() string               { return "test" }
func (r fakeRule) Check(*LintContext) []Violation { return nil }

type fakeReqRule struct {
	fakeRule
	mode CatalogMode
}

func (r fakeReqRule) RequiredCatalogMode() CatalogMode { return r.mode }

func TestDetectRequiredCatalogMode(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want CatalogMode
	}{
		{"plain", "def check():\n  return entities()", CatalogFast},
		{"refs_to", "def check():\n  for r in refs_to(x): pass", CatalogFull},
		{"refs_from", "def check():\n  refs_from(x)", CatalogFull},
		{"cycles", "def check():\n  for c in cycles(): pass", CatalogCommunities},
		{"module_dependencies", "def check():\n  module_dependencies()", CatalogCommunities},
		{"community_beats_refs", "def check():\n  refs_to(x); cycles()", CatalogCommunities},
		// A substring that isn't a call must not trigger (needs the "(").
		{"mention_only", "# refs_to is documented here", CatalogFast},
	}
	for _, tc := range cases {
		if got := detectRequiredCatalogMode(tc.src); got != tc.want {
			t.Errorf("%s: detectRequiredCatalogMode = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRequiredCatalogMode_MaxOverRules(t *testing.T) {
	rules := []Rule{
		fakeRule{id: "A"}, // no requirement
		fakeReqRule{fakeRule: fakeRule{id: "B"}, mode: CatalogFull},
		fakeReqRule{fakeRule: fakeRule{id: "C"}, mode: CatalogCommunities},
		fakeReqRule{fakeRule: fakeRule{id: "D"}, mode: CatalogFast},
	}
	if got := RequiredCatalogMode(rules); got != CatalogCommunities {
		t.Errorf("RequiredCatalogMode = %v, want communities (max)", got)
	}
	// A set with no requirers stays fast.
	if got := RequiredCatalogMode([]Rule{fakeRule{id: "X"}}); got != CatalogFast {
		t.Errorf("RequiredCatalogMode(no requirers) = %v, want fast", got)
	}
}

func TestLoadStarlarkRule_RequiredMode(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := t.TempDir() + "/rule.star"
		if err := osWriteFile(p, body); err != nil {
			t.Fatalf("write: %v", err)
		}
		return p
	}

	// Auto-detected from a refs_to call.
	r, err := LoadStarlarkRule(write(t, "RULE_ID='A'\ndef check():\n  refs_to('x')\n  return []\n"))
	if err != nil {
		t.Fatalf("load refs rule: %v", err)
	}
	if r.RequiredCatalogMode() != CatalogFull {
		t.Errorf("refs_to rule mode = %v, want full", r.RequiredCatalogMode())
	}

	// Explicit REQUIRES raises the mode even without a detectable builtin call.
	r2, err := LoadStarlarkRule(write(t, "RULE_ID='B'\nREQUIRES=['communities']\ndef check():\n  return []\n"))
	if err != nil {
		t.Fatalf("load REQUIRES rule: %v", err)
	}
	if r2.RequiredCatalogMode() != CatalogCommunities {
		t.Errorf("REQUIRES=['communities'] mode = %v, want communities", r2.RequiredCatalogMode())
	}

	// Plain rule stays fast.
	r3, err := LoadStarlarkRule(write(t, "RULE_ID='C'\ndef check():\n  return []\n"))
	if err != nil {
		t.Fatalf("load plain rule: %v", err)
	}
	if r3.RequiredCatalogMode() != CatalogFast {
		t.Errorf("plain rule mode = %v, want fast", r3.RequiredCatalogMode())
	}
}

func osWriteFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }
