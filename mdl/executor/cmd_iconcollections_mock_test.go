// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func mkIconCollection(mod *model.Module, name string, icons ...string) *types.IconCollection {
	ic := &types.IconCollection{
		BaseElement: model.BaseElement{ID: nextID("iconcol")},
		ContainerID: mod.ID,
		Name:        name,
		Prefix:      "mx-icon",
		ExportLevel: "Hidden",
	}
	for _, n := range icons {
		ic.Icons = append(ic.Icons, types.IconItem{Name: n})
	}
	return ic
}

func TestShowIconCollections_Mock(t *testing.T) {
	mod := mkModule("Theme")
	ic := mkIconCollection(mod, "Atlas_Filled", "pencil", "trash-can", "add")

	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:         func() bool { return true },
		ListIconCollectionsFunc: func() ([]*types.IconCollection, error) { return []*types.IconCollection{ic}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listIconCollections(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Icon Collection")
	assertContainsStr(t, out, "Theme.Atlas_Filled")
	assertContainsStr(t, out, "mx-icon") // prefix column
}

func TestDescribeIconCollection_Mock(t *testing.T) {
	mod := mkModule("Theme")
	ic := mkIconCollection(mod, "Atlas_Filled", "pencil", "add")

	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:         func() bool { return true },
		ListIconCollectionsFunc: func() ([]*types.IconCollection, error) { return []*types.IconCollection{ic}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeIconCollection(ctx, ast.QualifiedName{Module: "Theme", Name: "Atlas_Filled"}))

	out := buf.String()
	// lists icon names and the ready-to-use reference form
	assertContainsStr(t, out, "pencil")
	assertContainsStr(t, out, "Theme.Atlas_Filled.pencil")
}

func TestDescribeIconCollection_NotFound(t *testing.T) {
	mod := mkModule("Theme")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:         func() bool { return true },
		ListIconCollectionsFunc: func() ([]*types.IconCollection, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeIconCollection(ctx, ast.QualifiedName{Module: "Theme", Name: "NoSuch"}))
}
