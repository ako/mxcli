// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// placementProbe captures what a handler asked storage to reparent, so a test
// can assert on the move itself rather than on a rendering of it.
type placementProbe struct {
	moved       bool
	unitID      model.ID
	containerID model.ID
}

// folderMockBackend wires the minimum a CREATE OR MODIFY handler needs to
// resolve a folder: an existing module, a folder tree it can extend, and a
// recording MoveDocument.
func folderMockBackend(t *testing.T, mod *model.Module, probe *placementProbe) (*mock.MockBackend, *ContainerHierarchy) {
	t.Helper()
	folders := []*types.FolderInfo{}
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == mod.Name {
				return mod, nil
			}
			return nil, nil
		},
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return folders, nil },
		CreateFolderFunc: func(f *model.Folder) error {
			folders = append(folders, &types.FolderInfo{ID: f.ID, ContainerID: f.ContainerID, Name: f.Name})
			// Register the new folder so BuildFolderPath can walk up from it.
			withContainer(h, f.ID, f.ContainerID)
			h.folderNames[f.ID] = f.Name
			return nil
		},
		MoveDocumentFunc: func(unitID, containerID model.ID) error {
			probe.moved = true
			probe.unitID = unitID
			probe.containerID = containerID
			return nil
		},
	}
	return mb, h
}

// TestCreateOrModifyJsonStructureAppliesTheFolder is the reported case (#932):
// a FOLDER clause on a JSON structure that already exists was accepted,
// reported as a success, and dropped.
func TestCreateOrModifyJsonStructureAppliesTheFolder(t *testing.T) {
	mod := mkModule("RT")
	existing := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID, // module root
		Name:        "JSON_Example",
	}

	var probe placementProbe
	mb, h := folderMockBackend(t, mod, &probe)
	mb.ListJsonStructuresFunc = func() ([]*types.JsonStructure, error) {
		return []*types.JsonStructure{existing}, nil
	}
	mb.UpdateJsonStructureFunc = func(*types.JsonStructure) error { return nil }
	withContainer(h, existing.ContainerID, mod.ID)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateJsonStructure(ctx, &ast.CreateJsonStructureStmt{
		Name:           ast.QualifiedName{Module: "RT", Name: "JSON_Example"},
		Folder:         "Private/JSON Structures",
		JsonSnippet:    `{"result": 1}`,
		CreateOrModify: true,
	})
	assertNoError(t, err)

	if !probe.moved {
		t.Fatal("the folder clause was accepted but nothing was reparented")
	}
	if probe.unitID != existing.ID {
		t.Errorf("reparented %q, want the existing json structure %q", probe.unitID, existing.ID)
	}
	if probe.containerID == mod.ID || probe.containerID == "" {
		t.Errorf("reparented to %q, want the resolved folder rather than the module root", probe.containerID)
	}
}

// TestCreateOrModifyMicroflowAppliesTheFolder pins that this is a property of
// the class and not of JSON structures. The issue reported one doctype; every
// doctype with a FOLDER clause dropped it the same way, because the placement
// lives in the unit row that no Update* touches.
func TestCreateOrModifyMicroflowAppliesTheFolder(t *testing.T) {
	mod := mkModule("RT")
	existing := mkMicroflow(mod.ID, "ACT_Existing")

	var probe placementProbe
	mb, h := folderMockBackend(t, mod, &probe)
	mb.ListMicroflowsFunc = func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{existing}, nil }
	mb.UpdateMicroflowFunc = func(*microflows.Microflow) error { return nil }
	withContainer(h, existing.ContainerID, mod.ID)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateMicroflow(ctx, &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "RT", Name: "ACT_Existing"},
		Folder:         "Private",
		CreateOrModify: true,
	})
	assertNoError(t, err)

	if !probe.moved {
		t.Fatal("the folder clause was accepted but nothing was reparented")
	}
	if probe.unitID != existing.ID {
		t.Errorf("reparented %q, want the existing microflow %q", probe.unitID, existing.ID)
	}
}

// TestCreateOrModifyWithoutFolderLeavesPlacementAlone is the control, and the
// one that decides whether the fix is a fix or a new bug. An omitted FOLDER
// must keep the document where it is — a handler that reparented to the module
// root by default would yank every foldered document out of its folder on the
// next CREATE OR MODIFY, which is far worse than the silent no-op being fixed.
func TestCreateOrModifyWithoutFolderLeavesPlacementAlone(t *testing.T) {
	mod := mkModule("RT")
	folderID := nextID("fld")
	existing := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: folderID, // already filed away
		Name:        "JSON_Example",
	}

	var probe placementProbe
	mb, h := folderMockBackend(t, mod, &probe)
	mb.ListJsonStructuresFunc = func() ([]*types.JsonStructure, error) {
		return []*types.JsonStructure{existing}, nil
	}
	mb.UpdateJsonStructureFunc = func(*types.JsonStructure) error { return nil }
	withContainer(h, folderID, mod.ID)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateJsonStructure(ctx, &ast.CreateJsonStructureStmt{
		Name:           ast.QualifiedName{Module: "RT", Name: "JSON_Example"},
		JsonSnippet:    `{"result": 2}`,
		CreateOrModify: true,
	})
	assertNoError(t, err)

	if probe.moved {
		t.Errorf("a statement with no folder clause reparented the document to %q", probe.containerID)
	}
}

// TestApplyDocumentFolderSkipsAMoveToTheSamePlace pins that re-running a script
// that already placed a document does not touch the containment row. Storage
// elides it too, but the handler must not depend on that: a backend is allowed
// to be a plain writer, and ADR-0008's observable promise is that an in-sync
// project is left byte-identical.
func TestApplyDocumentFolderSkipsAMoveToTheSamePlace(t *testing.T) {
	folderID := nextID("fld")
	var probe placementProbe
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		MoveDocumentFunc: func(unitID, containerID model.ID) error {
			probe.moved = true
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	moved, err := applyDocumentFolder(ctx, nextID("unit"), folderID, folderID)
	assertNoError(t, err)
	if moved || probe.moved {
		t.Error("moving a document to the container it already occupies reached the backend")
	}
}
