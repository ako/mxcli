// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestShowBuildingBlocks_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	bb := mkBuildingBlock(mod.ID, "LoginForm")

	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBuildingBlocks(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.LoginForm")
	assertContainsStr(t, out, "(1 building blocks)")
}

func TestShowBuildingBlocks_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	bb1 := mkBuildingBlock(mod1.ID, "OrderCard")
	bb2 := mkBuildingBlock(mod2.ID, "EmployeeCard")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, bb1.ContainerID, mod1.ID)
	withContainer(h, bb2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb1, bb2}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBuildingBlocks(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.OrderCard")
	assertContainsStr(t, out, "HR.EmployeeCard")
}

func TestDescribeBuildingBlock_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{}, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeBuildingBlock(ctx, ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}
