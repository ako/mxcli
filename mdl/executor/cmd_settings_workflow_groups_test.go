// SPDX-License-Identifier: Apache-2.0

// ALTER SETTINGS WORKFLOWS ADD/MODIFY/REMOVE GROUP and SHOW WORKFLOW GROUPS —
// the workflow groups under App Settings ▸ Workflows ▸ Groups.
//
// Pinned against a real Mendix 11.13.0 project: the workflows settings part
// stores `Groups: [2]` (a typed array whose marker is 2, not the 3 the other
// settings child lists use), and each entry is
//
//	{$ID:…, $Type:"Settings$WorkflowGroup", Name:"Approvers", Description:"…"}
//
// and nothing else. Verified end to end by booting the app: the runtime
// materialises one system$workflowgroup row per entry, whose `modelguid` is the
// element's own $ID read as a .NET GUID — so the group's $ID is its database
// identity, and preserving it across a MODIFY is data safety, not tidiness.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

func wfGroupCtx(t *testing.T, ps *model.ProjectSettings) (*ExecContext, *bytes.Buffer, **model.ProjectSettings) {
	t.Helper()
	var written *model.ProjectSettings
	out := &bytes.Buffer{}
	b := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
		UpdateProjectSettingsFunc: func(p *model.ProjectSettings) error { written = p; return nil },
	}
	return &ExecContext{Backend: b, Output: out}, out, &written
}

func settingsWithGroups(groups ...model.WorkflowGroup) *model.ProjectSettings {
	return &model.ProjectSettings{Workflows: &model.WorkflowsSettings{
		UserEntity: "System.User", DefaultTaskParallelism: 3, WorkflowEngineParallelism: 5,
		Groups: groups,
	}}
}

func addGroupStmt(name string, props map[string]any) *ast.AlterSettingsStmt {
	if props == nil {
		props = map[string]any{}
	}
	return &ast.AlterSettingsStmt{Section: "workflows", AddGroup: true, GroupName: name, Properties: props}
}

func TestWorkflowGroupAdd_AppendsWithDescription(t *testing.T) {
	ps := settingsWithGroups()
	ctx, out, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps,
		addGroupStmt("Approvers", map[string]any{"Description": "Primary approval group"}))
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Workflows.Groups
	if len(got) != 1 || got[0].Name != "Approvers" || got[0].Description != "Primary approval group" {
		t.Fatalf("Groups = %+v, want one Approvers group with its description", got)
	}
	if !strings.Contains(out.String(), "Added workflow group: Approvers") {
		t.Errorf("output = %q", out.String())
	}
}

// Studio Pro appends rather than sorting, so the stored order is the order groups
// were added. The overlay rebuilds the list from this slice in the same order.
func TestWorkflowGroupAdd_AppendsInStatementOrder(t *testing.T) {
	ps := settingsWithGroups(model.WorkflowGroup{Name: "Approvers"})
	ctx, _, written := wfGroupCtx(t, ps)

	if err := alterSettingsWorkflowGroup(ctx, ps, addGroupStmt("Reviewers", nil)); err != nil {
		t.Fatal(err)
	}
	got := (*written).Workflows.Groups
	if len(got) != 2 || got[0].Name != "Approvers" || got[1].Name != "Reviewers" {
		t.Errorf("Groups = %+v, want [Approvers Reviewers]", got)
	}
}

func TestWorkflowGroupAdd_RefusesDuplicate(t *testing.T) {
	// Case-insensitively: Studio Pro will not let two groups differ only in case,
	// and a second 'approvers' would be a group no statement could address.
	ps := settingsWithGroups(model.WorkflowGroup{Name: "Approvers"})
	ctx, _, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps, addGroupStmt("approvers", nil))
	if err == nil {
		t.Fatal("adding an existing group was accepted")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v", err)
	}
	if *written != nil {
		t.Error("a refused ADD still wrote the settings")
	}
}

func TestWorkflowGroupAdd_RefusesUnknownOption(t *testing.T) {
	// Description is the only property Settings$WorkflowGroup declares. Storing
	// anything else would put a key in the document that the type does not have,
	// which mxbuild tolerates and Studio Pro refuses to open.
	ps := settingsWithGroups()
	ctx, _, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps,
		addGroupStmt("Approvers", map[string]any{"Descriptio": "typo"}))
	if err == nil {
		t.Fatal("an unknown group option was accepted")
	}
	if !strings.Contains(err.Error(), "unknown workflow group option") ||
		!strings.Contains(err.Error(), "Description") {
		t.Errorf("error = %v, want it to name the valid keys", err)
	}
	if *written != nil {
		t.Error("a refused ADD still wrote the settings")
	}
}

func TestWorkflowGroupAdd_RefusesBlankName(t *testing.T) {
	ps := settingsWithGroups()
	ctx, _, _ := wfGroupCtx(t, ps)
	if err := alterSettingsWorkflowGroup(ctx, ps, addGroupStmt("   ", nil)); err == nil {
		t.Fatal("a blank group name was accepted")
	}
}

func TestWorkflowGroupModify_TouchesOnlyNamedOptions(t *testing.T) {
	ps := settingsWithGroups(
		model.WorkflowGroup{Name: "Approvers", Description: "old"},
		model.WorkflowGroup{Name: "Reviewers", Description: "keep me"},
	)
	ctx, out, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps, &ast.AlterSettingsStmt{
		Section: "workflows", ModifyGroup: true, GroupName: "Approvers",
		Properties: map[string]any{"Description": "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Workflows.Groups
	if got[0].Description != "new" {
		t.Errorf("Approvers description = %q, want new", got[0].Description)
	}
	if got[1].Description != "keep me" {
		t.Errorf("MODIFY disturbed a sibling group: %+v", got[1])
	}
	if !strings.Contains(out.String(), "Modified workflow group: Approvers") {
		t.Errorf("output = %q", out.String())
	}
}

func TestWorkflowGroupModify_RefusesMissingGroup(t *testing.T) {
	ps := settingsWithGroups(model.WorkflowGroup{Name: "Approvers"})
	ctx, _, _ := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps, &ast.AlterSettingsStmt{
		Section: "workflows", ModifyGroup: true, GroupName: "Nope",
		Properties: map[string]any{"Description": "x"},
	})
	if err == nil {
		t.Fatal("modifying a group that does not exist was accepted")
	}
	// The error names what does exist: a script that mistypes a group should not
	// have to go and run `show workflow groups` to find out.
	if !strings.Contains(err.Error(), "Approvers") {
		t.Errorf("error = %v, want it to list the existing groups", err)
	}
}

// ADD OR MODIFY is what DESCRIBE emits, so replaying a described project must be
// quiet rather than an error — and must report "Unchanged" rather than claiming a
// write that ADR-0008 elides.
func TestWorkflowGroupUpsert_IsQuietOnReplay(t *testing.T) {
	ps := settingsWithGroups(model.WorkflowGroup{Name: "Approvers", Description: "d"})
	ctx, out, _ := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps, &ast.AlterSettingsStmt{
		Section: "workflows", UpsertGroup: true, GroupName: "Approvers",
		Properties: map[string]any{"Description": "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Unchanged workflow group: Approvers") {
		t.Errorf("output = %q, want the unchanged verb", out.String())
	}
}

func TestWorkflowGroupUpsert_AddsWhenAbsent(t *testing.T) {
	ps := settingsWithGroups()
	ctx, _, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps, &ast.AlterSettingsStmt{
		Section: "workflows", UpsertGroup: true, GroupName: "Approvers",
		Properties: map[string]any{"Description": "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := (*written).Workflows.Groups; len(got) != 1 || got[0].Name != "Approvers" {
		t.Errorf("Groups = %+v, want the group added", got)
	}
}

func TestWorkflowGroupRemove(t *testing.T) {
	ps := settingsWithGroups(
		model.WorkflowGroup{Name: "Approvers"},
		model.WorkflowGroup{Name: "Reviewers"},
	)
	ctx, out, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps,
		&ast.AlterSettingsStmt{Section: "workflows", RemoveGroup: true, GroupName: "Reviewers"})
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Workflows.Groups
	if len(got) != 1 || got[0].Name != "Approvers" {
		t.Errorf("Groups = %+v, want only Approvers", got)
	}
	if !strings.Contains(out.String(), "Removed workflow group: Reviewers") {
		t.Errorf("output = %q", out.String())
	}
}

func TestWorkflowGroupRemove_RefusesMissingGroup(t *testing.T) {
	ps := settingsWithGroups(model.WorkflowGroup{Name: "Approvers"})
	ctx, _, written := wfGroupCtx(t, ps)

	err := alterSettingsWorkflowGroup(ctx, ps,
		&ast.AlterSettingsStmt{Section: "workflows", RemoveGroup: true, GroupName: "Nope"})
	if err == nil {
		t.Fatal("removing a group that does not exist was accepted")
	}
	if *written != nil {
		t.Error("a refused REMOVE still wrote the settings")
	}
}

// GROUP outside the WORKFLOWS section is a mistake worth naming: the grammar
// shares the clause with the LANGUAGE forms, so `alter settings language add
// group 'X'` parses and would otherwise reach the language handler.
func TestAlterSettings_GroupOutsideWorkflowsIsRefused(t *testing.T) {
	ps := &model.ProjectSettings{
		Language: &model.LanguageSettings{DefaultLanguageCode: "en_US"},
		RawParts: []map[string]any{{"$Type": "Settings$LanguageSettings"}},
	}
	ctx, _, _ := wfGroupCtx(t, ps)
	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section: "language", AddGroup: true, GroupName: "X", Properties: map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "only defined for the WORKFLOWS section") {
		t.Fatalf("error = %v, want a refusal naming the section", err)
	}
}

func TestShowWorkflowGroups(t *testing.T) {
	ps := settingsWithGroups(
		model.WorkflowGroup{Name: "Approvers", Description: "Primary approval group"},
	)
	ctx, out, _ := wfGroupCtx(t, ps)

	if err := listWorkflowGroups(ctx); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Approvers") || !strings.Contains(s, "Primary approval group") {
		t.Errorf("output = %q", s)
	}
}
