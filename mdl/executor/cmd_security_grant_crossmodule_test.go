// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// checkDocumentAccessRolesSameModule is the shared CE0148 guard. Test both
// branches directly — it is pure, so no backend wiring is needed.
func TestCheckDocumentAccessRolesSameModule(t *testing.T) {
	sameModule := []ast.QualifiedName{{Module: "Sales", Name: "User"}, {Module: "Sales", Name: "Admin"}}
	if err := checkDocumentAccessRolesSameModule("page", "Sales", "Overview", sameModule); err != nil {
		t.Errorf("same-module roles should be accepted, got: %v", err)
	}

	crossModule := []ast.QualifiedName{{Module: "Sales", Name: "User"}, {Module: "HR", Name: "Manager"}}
	err := checkDocumentAccessRolesSameModule("page", "Sales", "Overview", crossModule)
	if err == nil {
		t.Fatal("cross-module role should be rejected")
	}
	msg := err.Error()
	for _, want := range []string{"CE0148", "Sales", "HR.Manager", "Overview"} {
		assertContainsStr(t, msg, want)
	}
}

// TestGrantPageAccess_CrossModuleRole_Rejected: granting a page a role from
// another module must fail with the CE0148 guard, before any write.
func TestGrantPageAccess_CrossModuleRole_Rejected(t *testing.T) {
	mod := mkModule("Sales")
	h := mkHierarchy(mod)
	pg := mkPage(mod.ID, "Overview")

	updated := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error {
			updated = true
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execGrantPageAccess(ctx, &ast.GrantPageAccessStmt{
		Page:  ast.QualifiedName{Module: "Sales", Name: "Overview"},
		Roles: []ast.QualifiedName{{Module: "HR", Name: "Manager"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "CE0148")
	assertContainsStr(t, err.Error(), "HR.Manager")
	if updated {
		t.Error("UpdateAllowedRoles must not be called when the grant is rejected")
	}
}

// TestGrantPageAccess_SameModuleRole_Succeeds is the regression guard: a
// same-module grant still works and writes the role.
func TestGrantPageAccess_SameModuleRole_Succeeds(t *testing.T) {
	mod := mkModule("Sales")
	h := mkHierarchy(mod)
	pg := mkPage(mod.ID, "Overview")

	var wrote []string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(moduleID model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error { wrote = roles; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execGrantPageAccess(ctx, &ast.GrantPageAccessStmt{
		Page:  ast.QualifiedName{Module: "Sales", Name: "Overview"},
		Roles: []ast.QualifiedName{{Module: "Sales", Name: "User"}},
	})
	assertNoError(t, err)
	found := false
	for _, r := range wrote {
		if r == "Sales.User" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Sales.User in written roles, got %v", wrote)
	}
}

// TestGrantMicroflowAccess_CrossModuleRole_Rejected mirrors the page case for a
// microflow (the guard is wired identically across all document types).
func TestGrantMicroflowAccess_CrossModuleRole_Rejected(t *testing.T) {
	mod := mkModule("Sales")
	h := mkHierarchy(mod)
	mf := mkMicroflow(mod.ID, "ACT_Do")

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error {
			t.Error("UpdateAllowedRoles must not be called for a rejected cross-module grant")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execGrantMicroflowAccess(ctx, &ast.GrantMicroflowAccessStmt{
		Microflow: ast.QualifiedName{Module: "Sales", Name: "ACT_Do"},
		Roles:     []ast.QualifiedName{{Module: "HR", Name: "Manager"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "CE0148")
}

// TestGrantNanoflowAccess_CrossModuleRole_Rejected mirrors the case for a nanoflow.
func TestGrantNanoflowAccess_CrossModuleRole_Rejected(t *testing.T) {
	mod := mkModule("Sales")
	h := mkHierarchy(mod)
	nf := mkNanoflow(mod.ID, "ACT_Nano")

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error {
			t.Error("UpdateAllowedRoles must not be called for a rejected cross-module grant")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "Sales", Name: "ACT_Nano"},
		Roles:    []ast.QualifiedName{{Module: "HR", Name: "Manager"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "CE0148")
}
