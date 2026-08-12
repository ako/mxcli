// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// allTools is the Studio Pro tool surface every table entry depends on, so a
// test that is not about tool presence gets a fully-equipped session.
var allTools = []string{
	"ped_create_document", "ped_create_module", "ped_update_document",
	"pg_read_page", "pg_patch_page",
}

func mendix(major, minor int) *types.ProjectVersion {
	return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
}

func TestCapabilityTableParses(t *testing.T) {
	feats := resolveCapabilities(mendix(11, 11), allTools, true)
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

// A missing tool must switch a feature off for the session, and say which tool.
// The table can no longer assert presence: 11.13 made the surface depend on the
// user's Studio Pro preferences and connected MCP servers (ADR-0006 Revision).
func TestResolveCapabilities_MissingToolBlocksFeature(t *testing.T) {
	var withoutPatch []string
	for _, tool := range allTools {
		if tool != "pg_patch_page" {
			withoutPatch = append(withoutPatch, tool)
		}
	}
	byKey := map[string]Capability{}
	for _, c := range resolveCapabilities(mendix(11, 13), withoutPatch, true) {
		byKey[c.Key] = c
	}
	pages := byKey["pages"]
	if pages.Available {
		t.Fatal("pages must be unavailable when pg_patch_page is absent")
	}
	if !strings.Contains(pages.Blocker, "pg_patch_page") {
		t.Fatalf("blocker should name the missing tool, got %q", pages.Blocker)
	}
	// Features that do not need that tool are untouched.
	if !byKey["entities"].Available {
		t.Error("entities must stay available; it does not depend on pg_patch_page")
	}
}

// A probe that did not answer means presence is unknown. Fail closed: a false
// "no" is the safe direction for a write path.
func TestResolveCapabilities_FailsClosedWhenProbeFailed(t *testing.T) {
	byKey := map[string]Capability{}
	for _, c := range resolveCapabilities(mendix(11, 13), nil, false) {
		byKey[c.Key] = c
	}
	if byKey["entities"].Available {
		t.Fatal("tool-dependent features must fail closed when tools/list did not answer")
	}
	if !strings.Contains(byKey["entities"].Blocker, "probe") {
		t.Fatalf("blocker should explain the probe failure, got %q", byKey["entities"].Blocker)
	}
}

// The version axis is the PROJECT's Mendix version. Keying on the MCP
// serverInfo.version was dead code — it is frozen at 1.0.0 across 11.11/11.12/
// 11.13, so no entry above the baseline could ever resolve true.
func TestProjectVersionAtLeast(t *testing.T) {
	cases := []struct {
		name string
		pv   *types.ProjectVersion
		want string
		ge   bool
	}{
		{"equal", mendix(11, 12), "11.12", true},
		{"newer minor", mendix(11, 13), "11.12", true},
		{"older minor", mendix(11, 11), "11.12", false},
		{"newer major", mendix(12, 0), "11.99", true},
		{"nil version stays blocked", nil, "11.12", false},
		{"unparseable entry blocks rather than gating on zero", mendix(11, 13), "eleven", false},
		{"single segment blocks", mendix(11, 13), "11", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := projectVersionAtLeast(c.pv, c.want); got != c.ge {
				t.Errorf("projectVersionAtLeast(%v, %q) = %v, want %v", c.pv, c.want, got, c.ge)
			}
		})
	}
}

func TestCanAuthorAndNotAuthorable(t *testing.T) {
	b := &Backend{capsCache: &Capabilities{
		Features: resolveCapabilities(mendix(11, 13), allTools, true),
	}}
	if !b.canAuthor("entities") {
		t.Error("entities should be authorable with the full tool surface")
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

// A session-specific blocker is more actionable than the generic table note, so
// it wins in the rejection message.
func TestNotAuthorable_PrefersSessionBlocker(t *testing.T) {
	b := &Backend{capsCache: &Capabilities{
		Features: resolveCapabilities(mendix(11, 13), nil, false),
	}}
	err := b.notAuthorable("page", "P", "pages")
	if err == nil || !strings.Contains(err.Error(), "probe") {
		t.Errorf("rejection should cite the session blocker, got %v", err)
	}
}

func TestCapabilityReport(t *testing.T) {
	b := &Backend{capsCache: &Capabilities{
		ProjectVersion: "11.13.0",
		ToolsProbed:    true,
		Tools:          allTools,
		FederatedTools: []string{"mcp_mendix-marketplace_Component_GetComponentIDsByCriteria"},
		Features:       resolveCapabilities(mendix(11, 13), allTools, true),
	}}
	r := b.CapabilityReport()
	for _, want := range []string{
		"MCP backend capabilities",
		"Project Mendix version: 11.13.0",
		"✓ Workflows —",        // authorable, from table
		"✗ Nanoflows — CREATE", // blocked, from a keyed entry
		"Studio Pro tools present",
		"Federated tools (1)",
		"never relies on them",
		"describes THIS session",
		"PED_MCP_CAPABILITIES.md",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("capability report missing %q in:\n%s", want, r)
		}
	}
	// A federated tool must not be counted among Studio Pro's own.
	if strings.Contains(r, "Studio Pro tools present (6)") {
		t.Error("federated tools must not inflate the Studio Pro tool count")
	}
}

func TestCapabilityReport_WarnsWhenProbeFailed(t *testing.T) {
	b := &Backend{capsCache: &Capabilities{
		Features: resolveCapabilities(nil, nil, false),
	}}
	r := b.CapabilityReport()
	if !strings.Contains(r, "tools/list did not answer") {
		t.Errorf("report must flag an unknown tool surface, got:\n%s", r)
	}
	if strings.Contains(r, "Studio Pro tools present") {
		t.Error("must not print a tool list when the probe did not answer")
	}
}
