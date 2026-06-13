// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

func TestCapabilityTableParses(t *testing.T) {
	feats := pedCapabilityFeatures("1.0.0")
	if len(feats) == 0 {
		t.Fatal("embedded capability table parsed to zero features")
	}
	avail := map[string]bool{}
	hasKey := map[string]bool{}
	for _, f := range feats {
		if f.Key == "" {
			t.Errorf("feature %q has no key", f.Feature)
		}
		avail[f.Key] = f.Available
		hasKey[f.Key] = true
	}
	if !avail["entities"] {
		t.Error("entities should be authorable at baseline")
	}
	for _, k := range []string{"nanoflow_create", "javaaction_create", "businessevent_create"} {
		if !hasKey[k] {
			t.Errorf("missing gated capability key %q", k)
		}
		if avail[k] {
			t.Errorf("%s should be blocked at baseline", k)
		}
	}
}

func TestServerVersionAtLeast(t *testing.T) {
	cases := []struct {
		have, want string
		ge         bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.2.0", "1.1.0", true},
		{"1.0.0", "1.1.0", false},
		{"2.0.0", "1.9.9", true},
		{"1.0", "1.0.1", false},
	}
	for _, c := range cases {
		if got := serverVersionAtLeast(c.have, c.want); got != c.ge {
			t.Errorf("serverVersionAtLeast(%q,%q) = %v, want %v", c.have, c.want, got, c.ge)
		}
	}
}

func TestCanAuthorAndNotAuthorable(t *testing.T) {
	b := &Backend{} // no connection -> baseline table (server version "")
	if !b.canAuthor("entities") {
		t.Error("entities should be authorable at baseline")
	}
	if b.canAuthor(capNanoflowCreate) {
		t.Error("nanoflow_create should be blocked at baseline")
	}
	if b.canAuthor("no_such_key") {
		t.Error("unknown capability key must default to not-authorable")
	}
	// notAuthorable sources its reason from the table note.
	err := b.notAuthorable("nanoflow", "NF", capNanoflowCreate)
	if err == nil || !strings.Contains(err.Error(), "create whitelist") {
		t.Errorf("notAuthorable should cite the table note, got %v", err)
	}
}

func TestCapabilityReport(t *testing.T) {
	r := (&Backend{}).CapabilityReport()
	for _, want := range []string{
		"MCP backend capabilities",
		"Studio Pro MCP server : (unknown) (unknown)",
		"✓ Workflows —",        // authorable, from table
		"✗ Nanoflows — CREATE", // blocked, from a now-split keyed entry
		"Reads (SHOW / DESCRIBE",
		"PED_MCP_CAPABILITIES.md",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("capability report missing %q in:\n%s", want, r)
		}
	}
	if strings.Contains(r, "PED tools present") {
		t.Error("no live client -> should not print a tool list")
	}
}
