// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1367: "marketplace update --save-edits + exec corrupts
// constant folder paths containing a literal '/' in the folder name". The
// Encryption module keeps EncryptionKey in `Private - String en/de-cryption`
// › `Apis`; the saved edit replayed it into `Private - String en` ›
// `de-cryption` › `Apis`, and exec exited 0.
//
// --save-edits writes DESCRIBE output, so the test is the round trip itself:
// describe the constant, parse what was written, resolve its folder in a
// project that does not have it yet, and compare the folders created with the
// ones described.
func TestDescribeExecRoundTrip_FolderNameContainingSlash(t *testing.T) {
	mod := mkModule("Encryption")
	outer := nextID("fld")
	inner := nextID("fld")
	c := mkConstant(inner, "EncryptionKey", "String", "")

	h := mkHierarchy(mod)
	withContainer(h, outer, mod.ID)
	withContainer(h, inner, outer)
	h.folderNames[outer] = "Private - String en/de-cryption"
	h.folderNames[inner] = "Apis"

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{c}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeConstant(ctx, ast.QualifiedName{Module: "Encryption", Name: "EncryptionKey"}))

	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, buf.String())
	}
	var folder string
	for _, s := range prog.Statements {
		if cs, ok := s.(*ast.CreateConstantStmt); ok {
			folder = cs.Folder
		}
	}
	if folder == "" {
		t.Fatalf("no folder clause in the describe output:\n%s", buf.String())
	}

	var probe placementProbe
	fresh, _ := folderMockBackend(t, mod, &probe)
	var created []*types.FolderInfo
	fresh.ListFoldersFunc = func() ([]*types.FolderInfo, error) { return created, nil }
	fresh.CreateFolderFunc = func(f *model.Folder) error {
		created = append(created, &types.FolderInfo{ID: f.ID, ContainerID: f.ContainerID, Name: f.Name})
		return nil
	}
	ctx2, _ := newMockCtx(t, withBackend(fresh), withHierarchy(mkHierarchy(mod)))
	if _, err := resolveFolder(ctx2, mod.ID, folder); err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, f := range created {
		names = append(names, f.Name)
	}
	want := []string{"Private - String en/de-cryption", "Apis"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("folder clause %q recreated folders %q, want %q", folder, names, want)
	}
	if len(created) == 2 && (created[0].ContainerID != mod.ID || created[1].ContainerID != created[0].ID) {
		t.Errorf("folders not nested as described: %+v", created)
	}
}
