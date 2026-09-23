// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#586 (follow-up): `create workflow … overview page X` reported
// success and stored NOTHING — the written unit carried no page reference and
// no "MyFirstModule.Overview" string anywhere. `mx check` passes, because a
// workflow with no overview page is valid, and `describe workflow` did not show
// it either, so nothing revealed the loss.
//
// Two halves, both measured against the Model SDK's own StructureVersionInfo
// (mendixmodelsdk 4.115.0, src/gen/workflows.js): `overviewPage` was DELETED in
// 9.11.0 and `adminPage` INTRODUCED in 9.11.0, so `AdminPage` — a
// Workflows$PageReference child — is the stored property, and
// generated/metamodel (the arbiter, an 11.6.0 snapshot) declares AdminPage and
// no OverviewPage at all.
//
//  1. WRITE: the executor set the semantic `Workflow.OverviewPage`, and the
//     backend only ever wrote `Workflow.AdminPage`. Two fields for one concept,
//     never joined — nothing copied one to the other.
//  2. READ: the reader took `g.OverviewPageQualifiedName()`, the pre-9.11
//     property, so even the `alter workflow … set overview page` path — which
//     writes AdminPage correctly — read back empty.

func createWorkflowWithOverviewPage(t *testing.T, b *Backend, containerID model.ID, name, page string) {
	t.Helper()
	wf := &workflows.Workflow{
		ContainerID:  containerID,
		Name:         name,
		WorkflowName: name,
		OverviewPage: page,
		Parameter:    &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"},
		Flow: &workflows.Flow{
			Activities: []workflows.WorkflowActivity{
				&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}},
				&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}},
			},
		},
	}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
}

// The written document must carry the page under AdminPage, as a
// Workflows$PageReference. Asserted on the raw unit rather than on a read-back,
// so a reader that is wrong in the same direction cannot make this pass.
func TestCreateWorkflowStoresOverviewPageAsAdminPage(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	createWorkflowWithOverviewPage(t, b, mod.ID, "ZzOverview", "MyFirstModule.Overview")

	wfs, err := b.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	var id model.ID
	for _, w := range wfs {
		if w.Name == "ZzOverview" {
			id = w.ID
		}
	}
	if id == "" {
		t.Fatal("workflow ZzOverview not found after create")
	}

	raw, err := b.GetRawUnit(id)
	if err != nil {
		t.Fatalf("GetRawUnit: %v", err)
	}
	admin, ok := raw["AdminPage"].(map[string]any)
	if !ok {
		t.Fatalf("AdminPage is %T, want a Workflows$PageReference document; keys = %v",
			raw["AdminPage"], sortedKeys(raw))
	}
	if got := admin["$Type"]; got != "Workflows$PageReference" {
		t.Errorf("AdminPage.$Type = %v, want Workflows$PageReference", got)
	}
	if got := admin["Page"]; got != "MyFirstModule.Overview" {
		t.Errorf("AdminPage.Page = %v, want MyFirstModule.Overview", got)
	}
	// The pre-9.11 spelling must NOT also be written — writing both as a hedge
	// is what CLAUDE.md's overlay rule forbids, and Studio Pro resolves every
	// stored property against the type's property list.
	if _, present := raw["OverviewPage"]; present {
		t.Errorf("OverviewPage was written as well as AdminPage: %v", raw["OverviewPage"])
	}
}

// And it must read back, so describe → exec round-trips the clause.
func TestReadWorkflowOverviewPageFromAdminPage(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	createWorkflowWithOverviewPage(t, b, mod.ID, "ZzOverviewRead", "MyFirstModule.Overview")

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	wfs, err := b2.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	for _, w := range wfs {
		if w.Name != "ZzOverviewRead" {
			continue
		}
		if w.OverviewPage != "MyFirstModule.Overview" {
			t.Fatalf("OverviewPage read back as %q, want MyFirstModule.Overview", w.OverviewPage)
		}
		return
	}
	t.Fatal("workflow ZzOverviewRead not found")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
