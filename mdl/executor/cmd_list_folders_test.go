// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mxcli-formula1 #32: MOVE could place a document in a folder, but nothing could
// read the placement back — SHOW STRUCTURE is flat at every depth and DESCRIBE
// reports one document at a time. So a move could not be confirmed, and an
// intended layout could not be diffed against the real one, without opening the
// .mpr as SQLite.
func TestListFolders(t *testing.T) {
	ctx, buf := foldersFixture(t)

	assertNoError(t, listFolders(ctx, &ast.ShowStmt{ObjectType: ast.ShowFolders, InModule: "Mv"}))
	out := buf.String()

	// A nested path is one row per level, so the layout reads as a tree.
	assertContainsStr(t, out, "Api/Published")
	assertContainsStr(t, out, "ODataService Api")
	assertContainsStr(t, out, "Support")
	assertContainsStr(t, out, "JavaAction Helper")

	// An empty folder is part of the layout: a listing that hid it could not
	// round-trip against an intended one.
	if !strings.Contains(out, "Api  [0]") {
		t.Errorf("the empty intermediate folder is missing:\n%s", out)
	}

	// A document still at the module root is the thing you most want to notice,
	// so it appears in the same view rather than by subtraction.
	assertContainsStr(t, out, "(module root)")
	assertContainsStr(t, out, "Microflow Unfiled")
}

// The IN clause scopes the listing, case-insensitively as MDL is elsewhere.
func TestListFolders_ModuleFilter(t *testing.T) {
	for _, filter := range []string{"Mv", "mv", "MV"} {
		ctx, buf := foldersFixture(t)
		assertNoError(t, listFolders(ctx, &ast.ShowStmt{ObjectType: ast.ShowFolders, InModule: filter}))
		if out := buf.String(); !strings.Contains(out, "Support") || strings.Contains(out, "Other") {
			t.Errorf("filter %q listed the wrong modules:\n%s", filter, out)
		}
	}

	// A module with no folders and no documents says so rather than printing an
	// empty listing that reads like a broken command.
	ctx, buf := foldersFixture(t)
	assertNoError(t, listFolders(ctx, &ast.ShowStmt{ObjectType: ast.ShowFolders, InModule: "Nope"}))
	assertContainsStr(t, buf.String(), "No folders or documents in Nope")
}

// Two runs over the same model must render identically, or a diff against a
// checked-in layout shows movement that did not happen.
func TestListFolders_Deterministic(t *testing.T) {
	ctx1, buf1 := foldersFixture(t)
	assertNoError(t, listFolders(ctx1, &ast.ShowStmt{ObjectType: ast.ShowFolders}))
	ctx2, buf2 := foldersFixture(t)
	assertNoError(t, listFolders(ctx2, &ast.ShowStmt{ObjectType: ast.ShowFolders}))
	if buf1.String() != buf2.String() {
		t.Errorf("output is not stable across runs:\n--- 1 ---\n%s\n--- 2 ---\n%s", buf1.String(), buf2.String())
	}
}

// foldersFixture builds a two-module model: Mv with Support and Api/Published
// (the second nested under an otherwise empty Api), one document in each, one
// microflow left at the module root, and an unrelated module to test the filter.
func foldersFixture(t *testing.T) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	mv := mkModule("Mv")
	other := mkModule("Other")

	support := &types.FolderInfo{ID: "f-support", ContainerID: mv.ID, Name: "Support"}
	api := &types.FolderInfo{ID: "f-api", ContainerID: mv.ID, Name: "Api"}
	published := &types.FolderInfo{ID: "f-pub", ContainerID: api.ID, Name: "Published"}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mv, other}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) {
			return []*types.FolderInfo{support, api, published}, nil
		},
		ListUnitsFunc: func() ([]*types.UnitInfo, error) { return nil, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{{Name: "Unfiled", ContainerID: mv.ID}}, nil
		},
		ListJavaActionsFullFunc: func() ([]*javaactions.JavaAction, error) {
			return []*javaactions.JavaAction{{Name: "Helper", ContainerID: support.ID}}, nil
		},
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) {
			return []*model.PublishedODataService{{Name: "Api", ContainerID: published.ID}}, nil
		},
	}

	// No stub hierarchy: the listing's whole job is to read placement back out of
	// the model, so it is built from the same three backend calls production uses.
	ctx, buf := newMockCtx(t, withBackend(mb))
	return ctx, buf
}

// #892: the folder holding Mendix's own FeedbackModule mappings rendered as
// `[0]` because documentsByContainer named twelve document kinds and mappings
// and JSON structures were not among them. That empty count is what made
// dropping the folder look safe.
func TestListFolders_CountsMappingsAndJsonStructures(t *testing.T) {
	mod := mkModule("Fb")
	mappings := &types.FolderInfo{ID: "f-map", ContainerID: mod.ID, Name: "Mappings"}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) {
			return []*types.FolderInfo{mappings}, nil
		},
		ListUnitsFunc: func() ([]*types.UnitInfo, error) { return nil, nil },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) {
			return []*types.JsonStructure{{Name: "JSON_Response", ContainerID: mappings.ID}}, nil
		},
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) {
			return []*model.ImportMapping{{Name: "IMM_PostResponse", ContainerID: mappings.ID}}, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			return []*model.ExportMapping{{Name: "EXM_PostFeedback", ContainerID: mappings.ID}}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listFolders(ctx, &ast.ShowStmt{ObjectType: ast.ShowFolders, InModule: "Fb"}))
	out := buf.String()

	if strings.Contains(out, "Mappings  [0]") {
		t.Errorf("folder still reports [0] while holding three documents — this is the count that made #892 look safe:\n%s", out)
	}
	assertContainsStr(t, out, "JsonStructure JSON_Response")
	assertContainsStr(t, out, "ImportMapping IMM_PostResponse")
	assertContainsStr(t, out, "ExportMapping EXM_PostFeedback")
}
