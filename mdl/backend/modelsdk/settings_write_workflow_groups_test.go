// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// workflowsPart returns the Settings$WorkflowsProjectSettingsPart of a settings
// document's raw parts. The fixture carries one, as every Mendix 11 project does.
func workflowsPart(t *testing.T, ps *model.ProjectSettings) map[string]any {
	t.Helper()
	for _, p := range ps.RawParts {
		if p["$Type"] == "Settings$WorkflowsProjectSettingsPart" {
			return p
		}
	}
	t.Fatalf("no Settings$WorkflowsProjectSettingsPart in the settings document")
	return nil
}

// TestUpdateProjectSettings_WorkflowGroupsReachStorage is the write-path proof.
//
// The failure it exists to catch is silent: without the Groups overlay in
// UpdateProjectSettings the executor reports "Added workflow group: Approvers"
// and the stored Groups list is carried through from the preserved part
// unchanged, so nothing is written. That is the shape the enabled-language list
// already went through, and no `mx check` reports it — the document stays valid,
// it just does not have the group in it.
func TestUpdateProjectSettings_WorkflowGroupsReachStorage(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	if ps.Workflows == nil {
		t.Fatalf("workflows settings not read")
	}
	if len(ps.Workflows.Groups) != 0 {
		t.Fatalf("fixture already has groups: %+v", ps.Workflows.Groups)
	}
	partCount := len(ps.RawParts)

	ps.Workflows.Groups = []model.WorkflowGroup{
		{Name: "Approvers", Description: "Primary approval group"},
		{Name: "Reviewers"},
	}
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	ps2, err := b2.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings(2): %v", err)
	}
	got := ps2.Workflows.Groups
	if len(got) != 2 {
		t.Fatalf("Groups = %+v, want the two written groups", got)
	}
	if got[0].Name != "Approvers" || got[0].Description != "Primary approval group" {
		t.Errorf("group[0] = %+v", got[0])
	}
	if got[1].Name != "Reviewers" || got[1].Description != "" {
		t.Errorf("group[1] = %+v, want Reviewers with no description", got[1])
	}
	if len(ps2.RawParts) != partCount {
		t.Errorf("part count changed: was %d, now %d (the overlay dropped a part)", partCount, len(ps2.RawParts))
	}

	// The stored list keeps the marker the project already carried. A blank
	// 11.13.0 project stores `Groups: [2]`, not the 3 the other settings child
	// lists use, and rebuilding one with the wrong marker silently downgrades it.
	raw := workflowsPart(t, ps2)
	arr, ok := raw["Groups"].(bson.A)
	if !ok || len(arr) != 3 {
		t.Fatalf("stored Groups = %#v, want marker + two groups", raw["Groups"])
	}
	if m, _ := arr[0].(int32); m != 2 {
		t.Errorf("stored Groups marker = %#v, want 2 (the marker the fixture carried)", arr[0])
	}
	// Exactly the four keys the type declares. mxbuild tolerates a fifth; Studio
	// Pro throws at MprProperty.cs and will not open the project.
	// The re-read document decodes into bson.M, which is a map[string]any under a
	// different name, so both spellings have to be accepted here.
	var g map[string]any
	switch v := arr[1].(type) {
	case bson.M:
		g = v
	case map[string]any:
		g = v
	default:
		t.Fatalf("stored group = %#v, want a document", arr[1])
	}
	for k := range g {
		switch k {
		case "$ID", "$Type", "Name", "Description":
		default:
			t.Errorf("stored group carries undeclared key %q = %#v", k, g[k])
		}
	}
	if g["$Type"] != "Settings$WorkflowGroup" {
		t.Errorf("stored group $Type = %v", g["$Type"])
	}
}

// TestUpdateProjectSettings_WorkflowGroupKeepsItsStoredID is the data-safety half.
//
// A group's element $ID is its identity in the runtime database: booting the app
// materialises one system$workflowgroup row per entry whose `modelguid` is that
// $ID read as a .NET GUID (measured on Mendix 11.13.0 — stored $ID bytes
// 7c5fc4cf05c3394fa6718a3d4603e9a5, row modelguid
// cfc45f7c-c305-4f39-a671-8a3d4603e9a5). Minting a fresh one on a description
// edit would make the runtime treat it as a different group, orphaning the
// system$workflowgroup_user memberships and the user tasks already targeting it —
// with a perfectly valid model, so no build and no check would report it. Same
// class as the entity GUID case in CLAUDE.md.
func TestUpdateProjectSettings_WorkflowGroupKeepsItsStoredID(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, _ := b.GetProjectSettings()
	ps.Workflows.Groups = []model.WorkflowGroup{{Name: "Approvers", Description: "old"}}
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}

	ps2, _ := b.GetProjectSettings()
	if len(ps2.Workflows.Groups) != 1 {
		t.Fatalf("Groups = %+v, want the written group back", ps2.Workflows.Groups)
	}
	before := ps2.Workflows.Groups[0].ID
	if before == "" {
		t.Fatalf("stored group has no $ID")
	}

	ps2.Workflows.Groups[0].Description = "new"
	if err := b.UpdateProjectSettings(ps2); err != nil {
		t.Fatalf("UpdateProjectSettings(2): %v", err)
	}

	ps3, _ := b.GetProjectSettings()
	if len(ps3.Workflows.Groups) != 1 {
		t.Fatalf("Groups = %+v, want the group back after the edit", ps3.Workflows.Groups)
	}
	if got := ps3.Workflows.Groups[0]; got.ID != before {
		t.Errorf("group $ID changed on a description edit: %v -> %v", before, got.ID)
	} else if got.Description != "new" {
		t.Errorf("Description = %q, want the edit to have landed", got.Description)
	}
}

// TestUpdateProjectSettings_RefusesUnreadableGroupList is guard-don't-drop
// (ADR-0005): the Groups list is rebuilt from the semantic slice, so an element
// the read could not convert would be deleted by any rewrite — including one that
// only meant to change a description. Refuse instead of losing it.
func TestUpdateProjectSettings_RefusesUnreadableGroupList(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, _ := b.GetProjectSettings()
	ps.Workflows.GroupsIncomplete = true
	ps.Workflows.Groups = []model.WorkflowGroup{{Name: "Approvers"}}

	err := b.UpdateProjectSettings(ps)
	if err == nil {
		t.Fatal("a rewrite over an unreadable group list was accepted")
	}
	// The control: with the flag clear the very same write goes through, so the
	// refusal is the flag's doing and not something else about this document.
	ps.Workflows.GroupsIncomplete = false
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("the same write was refused with the flag clear: %v", err)
	}
}
