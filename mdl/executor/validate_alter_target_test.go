// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `alter page Mod.NoSuchPage` passed `mxcli check -p --references` and was then
// refused by exec with "page not found" — check was the weaker gate, which
// inverts the contract the check-syntax skill states and lets a script stop
// halfway through, with the statements before the typo already applied
// (ako/mxcli-rest FINDINGS #60).

func alterTargetFixture(t *testing.T) *ExecContext {
	t.Helper()
	mod := &model.Module{Name: "Shop"}
	mod.ID = nextID("mod")

	page := &pages.Page{Name: "Home_Web"}
	page.ID = nextID("page")
	page.ContainerID = mod.ID

	snippet := &pages.Snippet{Name: "Sn_Header"}
	snippet.ID = nextID("snip")
	snippet.ContainerID = mod.ID

	layout := &pages.Layout{Name: "App_Default"}
	layout.ID = nextID("layout")
	layout.ContainerID = mod.ID

	ent := &domainmodel.Entity{Name: "Order", Persistable: true}
	ent.ID = nextID("ent")
	dm := &domainmodel.DomainModel{ContainerID: mod.ID, Entities: []*domainmodel.Entity{ent}}
	dm.ID = nextID("dm")

	h := mkHierarchy(mod)
	withContainer(h, page.ID, mod.ID)
	withContainer(h, snippet.ID, mod.ID)
	withContainer(h, layout.ID, mod.ID)
	withContainer(h, dm.ID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListPagesFunc:        func() ([]*pages.Page, error) { return []*pages.Page{page}, nil },
		ListSnippetsFunc:     func() ([]*pages.Snippet, error) { return []*pages.Snippet{snippet}, nil },
		ListLayoutsFunc:      func() ([]*pages.Layout, error) { return []*pages.Layout{layout}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func TestValidateAlterTarget_ReportsAMissingDocument(t *testing.T) {
	ctx := alterTargetFixture(t)

	for _, tc := range []struct {
		name string
		stmt ast.Statement
		want string // the kind named in the error
		near string // a real name in the module, which the error should offer
	}{
		{"page", &ast.AlterPageStmt{ContainerType: "PAGE",
			PageName: ast.QualifiedName{Module: "Shop", Name: "Hoem_Web"}}, "page", "Home_Web"},
		{"snippet", &ast.AlterPageStmt{ContainerType: "SNIPPET",
			PageName: ast.QualifiedName{Module: "Shop", Name: "Sn_Heade"}}, "snippet", "Sn_Header"},
		{"layout", &ast.AlterPageStmt{ContainerType: "LAYOUT",
			PageName: ast.QualifiedName{Module: "Shop", Name: "App_Defualt"}}, "layout", "App_Default"},
		{"entity", &ast.AlterEntityStmt{
			Name: ast.QualifiedName{Module: "Shop", Name: "Ordr"}}, "entity", "Order"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAlterTarget(ctx, tc.stmt, newScriptContext())
			if err == nil {
				t.Fatal("a misspelled target was accepted — exec would refuse it, so check is the weaker gate")
			}
			if !strings.Contains(err.Error(), tc.want+" not found") {
				t.Errorf("the error should name the kind %q: %v", tc.want, err)
			}
			// A typo is much cheaper to fix when the error shows what is there.
			if !strings.Contains(err.Error(), tc.near) {
				t.Errorf("the error should offer %q as a near name: %v", tc.near, err)
			}
		})
	}
}

// CONTROL: the targets that DO exist must pass. Without this a validator that
// always returned an error would satisfy the test above.
func TestValidateAlterTarget_AcceptsAnExistingDocument(t *testing.T) {
	ctx := alterTargetFixture(t)

	for _, stmt := range []ast.Statement{
		&ast.AlterPageStmt{ContainerType: "PAGE", PageName: ast.QualifiedName{Module: "Shop", Name: "Home_Web"}},
		&ast.AlterPageStmt{ContainerType: "SNIPPET", PageName: ast.QualifiedName{Module: "Shop", Name: "Sn_Header"}},
		&ast.AlterPageStmt{ContainerType: "LAYOUT", PageName: ast.QualifiedName{Module: "Shop", Name: "App_Default"}},
		&ast.AlterEntityStmt{Name: ast.QualifiedName{Module: "Shop", Name: "Order"}},
	} {
		if err := validateAlterTarget(ctx, stmt, newScriptContext()); err != nil {
			t.Errorf("an existing target was rejected: %v", err)
		}
	}
}

// CONTROL: a script that creates the document and then alters it is the common
// shape, and must not be broken by a check that only looks at the project.
func TestValidateAlterTarget_CountsDocumentsTheScriptCreates(t *testing.T) {
	ctx := alterTargetFixture(t)

	sc := newScriptContext()
	sc.pages["Shop.P_New"] = true
	sc.snippets["Shop.Sn_New"] = true
	sc.layouts["Shop.L_New"] = true
	sc.entities["Shop.NewThing"] = true

	for _, stmt := range []ast.Statement{
		&ast.AlterPageStmt{ContainerType: "PAGE", PageName: ast.QualifiedName{Module: "Shop", Name: "P_New"}},
		&ast.AlterPageStmt{ContainerType: "SNIPPET", PageName: ast.QualifiedName{Module: "Shop", Name: "Sn_New"}},
		&ast.AlterPageStmt{ContainerType: "LAYOUT", PageName: ast.QualifiedName{Module: "Shop", Name: "L_New"}},
		&ast.AlterEntityStmt{Name: ast.QualifiedName{Module: "Shop", Name: "NewThing"}},
	} {
		if err := validateAlterTarget(ctx, stmt, sc); err != nil {
			t.Errorf("a document the script creates was reported missing: %v", err)
		}
	}
}

// CONTROL: a module the script creates has no listing to resolve against, so
// its documents cannot be checked and must not be reported.
func TestValidateAlterTarget_SkipsAModuleTheScriptCreates(t *testing.T) {
	ctx := alterTargetFixture(t)
	sc := newScriptContext()
	sc.modules["Fresh"] = true

	stmt := &ast.AlterPageStmt{ContainerType: "PAGE",
		PageName: ast.QualifiedName{Module: "Fresh", Name: "Anything"}}
	if err := validateAlterTarget(ctx, stmt, sc); err != nil {
		t.Errorf("a document in a script-created module was reported missing: %v", err)
	}
}

// CONTROL: an empty listing means the backend could not answer, not that the
// project has no pages. Reporting "not found" from it would fail every script
// against a backend that does not implement the listing.
func TestValidateAlterTarget_SilentWhenTheListingIsEmpty(t *testing.T) {
	mod := &model.Module{Name: "Shop"}
	mod.ID = nextID("mod")
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		ListPagesFunc:       func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	stmt := &ast.AlterPageStmt{ContainerType: "PAGE",
		PageName: ast.QualifiedName{Module: "Shop", Name: "Anything"}}
	if err := validateAlterTarget(ctx, stmt, newScriptContext()); err != nil {
		t.Errorf("an unanswerable listing produced a not-found: %v", err)
	}
}
