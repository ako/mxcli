// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// alterPageWith runs one ALTER PAGE operation against a mock mutator.
func alterPageWith(t *testing.T, mutator *mock.MockPageMutator, op ast.AlterPageOperation) error {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	if mutator.SaveFunc == nil {
		mutator.SaveFunc = func() error { return nil }
	}
	// The real mutator hands back live scopes; the mock returns nil maps unless
	// configured, and the builder writes into them.
	if mutator.WidgetScopeFunc == nil {
		mutator.WidgetScopeFunc = func() map[string]model.ID { return map[string]model.ID{} }
	}
	if mutator.ParamScopeFunc == nil {
		mutator.ParamScopeFunc = func() (map[string]model.ID, map[string]string) {
			return map[string]model.ID{}, map[string]string{}
		}
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return mutator, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return execAlterPage(ctx, &ast.AlterPageStmt{
		PageName:   ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{op},
	})
}

func templateNode(specialization string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:           "template",
		Specialization: specialization,
		Properties:     map[string]any{},
		Children: []*ast.WidgetV3{{
			Type: "dynamictext", Name: "lbl", Properties: map[string]any{"Content": "x"},
		}},
	}
}

// TestAlterPageInsertTemplateRoutesToTemplates is the routing assertion.
//
// INSERT INTO normally appends to a container's Widgets. A list view's Widgets
// is its DEFAULT BODY, and a Forms$ListViewTemplate is not a widget — so a
// template routed through InsertWidget would append a non-widget to the widget
// list, producing a page Studio Pro cannot open. It must take the dedicated
// path, the same way DataGrid2 columns do.
func TestAlterPageInsertTemplateRoutesToTemplates(t *testing.T) {
	var gotListView string
	var gotTemplates []*pages.ListViewTemplate
	insertWidgetCalled := false

	mutator := &mock.MockPageMutator{
		EnclosingEntityForChildrenFunc: func(widgetRef string) string { return "" },
		InsertListViewTemplatesFunc: func(listViewRef string, templates []*pages.ListViewTemplate) error {
			gotListView = listViewRef
			gotTemplates = templates
			return nil
		},
		InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
			insertWidgetCalled = true
			return nil
		},
	}

	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "vehicleListView"},
		Widgets:  []*ast.WidgetV3{templateNode("Pages.Bus")},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if insertWidgetCalled {
		t.Error("a template went through InsertWidget — it would land in the list view's default body")
	}
	if gotListView != "vehicleListView" {
		t.Errorf("listViewRef = %q, want vehicleListView", gotListView)
	}
	if len(gotTemplates) != 1 || gotTemplates[0].Specialization != "Pages.Bus" {
		t.Fatalf("templates = %+v, want one for Pages.Bus", gotTemplates)
	}
	if gotTemplates[0].TypeName != "Forms$ListViewTemplate" {
		t.Errorf("TypeName = %q", gotTemplates[0].TypeName)
	}
	if len(gotTemplates[0].Widgets) != 1 {
		t.Errorf("template has %d widget(s), want 1", len(gotTemplates[0].Widgets))
	}
}

// TestAlterPageInsertTemplateRefusals covers the shapes that cannot mean what
// they look like.
func TestAlterPageInsertTemplateRefusals(t *testing.T) {
	newMutator := func() *mock.MockPageMutator {
		return &mock.MockPageMutator{
			EnclosingEntityForChildrenFunc: func(string) string { return "" },
			InsertListViewTemplatesFunc: func(string, []*pages.ListViewTemplate) error {
				return nil
			},
		}
	}

	t.Run("BEFORE/AFTER is refused", func(t *testing.T) {
		err := alterPageWith(t, newMutator(), &ast.InsertWidgetOp{
			Position: "AFTER",
			Target:   ast.WidgetRef{Widget: "someWidget"},
			Widgets:  []*ast.WidgetV3{templateNode("Pages.Bus")},
		})
		assertError(t, err)
		assertContainsStr(t, err.Error(), "INSERT INTO")
	})

	t.Run("mixing templates and widgets is refused", func(t *testing.T) {
		err := alterPageWith(t, newMutator(), &ast.InsertWidgetOp{
			Position: "INTO",
			Target:   ast.WidgetRef{Widget: "vehicleListView"},
			Widgets: []*ast.WidgetV3{
				{Type: "dynamictext", Name: "plain", Properties: map[string]any{"Content": "x"}},
				templateNode("Pages.Bus"),
			},
		})
		assertError(t, err)
		assertContainsStr(t, err.Error(), "different places")
	})

	t.Run("two templates for one entity in one INSERT is refused", func(t *testing.T) {
		err := alterPageWith(t, newMutator(), &ast.InsertWidgetOp{
			Position: "INTO",
			Target:   ast.WidgetRef{Widget: "vehicleListView"},
			Widgets:  []*ast.WidgetV3{templateNode("Pages.Bus"), templateNode("Pages.Bus")},
		})
		assertError(t, err)
		assertContainsStr(t, err.Error(), "two templates for Pages.Bus")
	})
}

// TestAlterPageDropTemplateReachesTheMutator pins the dispatch for the operation
// that needed new syntax, since a template has no name to put in a widgetRef.
func TestAlterPageDropTemplateReachesTheMutator(t *testing.T) {
	var gotListView, gotSpec string
	mutator := &mock.MockPageMutator{
		DropListViewTemplateFunc: func(listViewRef, specialization string) error {
			gotListView, gotSpec = listViewRef, specialization
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.DropListViewTemplateOp{
		Specialization: "Pages.SUV",
		ListView:       "vehicleListView",
	})
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	if gotListView != "vehicleListView" || gotSpec != "Pages.SUV" {
		t.Errorf("DropListViewTemplate(%q, %q), want (vehicleListView, Pages.SUV)", gotListView, gotSpec)
	}
}
