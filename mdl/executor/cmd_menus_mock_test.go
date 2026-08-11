// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// menuBackend returns a MockBackend serving one menu document.
func menuBackend(md *types.MenuDocument) *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetMenuDocumentByQualifiedNameFunc: func(moduleName, name string) (*types.MenuDocument, error) {
			if md != nil && md.Name == name {
				return md, nil
			}
			return nil, fmt.Errorf("menu not found: %s.%s", moduleName, name)
		},
	}
}

// TestDescribeMenu_Nested is the renderer's real exercise. The vendored fixture's
// Atlas menus are flat — every MenuItem's Items array holds only the list marker
// — so recursion, page/microflow targets and the non-round-trippable icon note
// have no coverage there. This builds a menu that has all of them.
func TestDescribeMenu_Nested(t *testing.T) {
	md := &types.MenuDocument{
		Name:        "Main_Menu",
		ExportLevel: "Hidden",
		Items: []*types.NavMenuItem{
			{
				Caption:  "Home",
				Page:     "MyModule.Home_Web",
				IconType: "Forms$IconCollectionIcon",
				Icon:     "Atlas_Core.Atlas.home",
			},
			{
				Caption: "Admin",
				Items: []*types.NavMenuItem{
					{Caption: "Accounts", Page: "Administration.Account_Overview"},
					{Caption: "Rebuild", Microflow: "Administration.Rebuild"},
				},
			},
			{
				// A glyph icon carries a numeric Code and no name, so it cannot be
				// expressed in MDL. It must be flagged, not silently dropped.
				Caption:  "Settings",
				IconType: "Forms$GlyphIcon",
			},
		},
	}

	ctx, buf := newMockCtx(t, withBackend(menuBackend(md)))
	assertNoError(t, describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Main_Menu"}))
	out := buf.String()

	// The output is re-executable, so it opens with the statement that recreates it.
	assertContainsStr(t, out, "create or modify menu Atlas_Core.Main_Menu (")
	assertContainsStr(t, out, "menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;")

	// A sub-menu opens a nested block and its children are indented one level in.
	assertContainsStr(t, out, "menu 'Admin' (")
	assertContainsStr(t, out, "    menu item 'Accounts' page Administration.Account_Overview;")
	assertContainsStr(t, out, "    menu item 'Rebuild' microflow Administration.Rebuild;")

	// The glyph icon is reported rather than dropped, and points at the statement
	// that would have to reproduce it — CREATE MENU, not CREATE NAVIGATION.
	assertContainsStr(t, out, "is not reproducible by CREATE MENU")
	if strings.Contains(out, "CREATE NAVIGATION") {
		t.Errorf("menu output should not point at CREATE NAVIGATION, which authors a profile menu, not a menu document:\n%s", out)
	}
}

func TestDescribeMenu_Empty(t *testing.T) {
	md := &types.MenuDocument{Name: "Empty_Menu"}
	ctx, buf := newMockCtx(t, withBackend(menuBackend(md)))
	assertNoError(t, describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Empty_Menu"}))
	// An empty menu still describes to a statement that recreates it.
	assertContainsStr(t, buf.String(), "create or modify menu Atlas_Core.Empty_Menu (")
	assertContainsStr(t, buf.String(), ");")
}

func TestDescribeMenu_NotFound(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(menuBackend(nil)))
	err := describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Nope"})
	if err == nil {
		t.Fatal("expected an error for a menu that does not exist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to report the menu was not found", err)
	}
}

// TestDescribeMenu_Documentation checks the doc comment is emitted, since a
// menu document carries Documentation like any other document.
func TestDescribeMenu_Documentation(t *testing.T) {
	md := &types.MenuDocument{Name: "Doc_Menu", Documentation: "Phone navigation."}
	ctx, buf := newMockCtx(t, withBackend(menuBackend(md)))
	assertNoError(t, describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Doc_Menu"}))
	assertContainsStr(t, buf.String(), "Phone navigation.")
}

// TestCreateMenu_RejectsDuplicateWithoutOrModify pins the create/modify split:
// a plain CREATE against an existing menu must fail rather than silently
// replacing a document the author did not mean to touch.
func TestCreateMenu_RejectsDuplicateWithoutOrModify(t *testing.T) {
	existing := &types.MenuDocument{ID: "menu-1", Name: "Main_Menu"}
	mb := menuBackend(existing)
	mb.GetModuleByNameFunc = func(name string) (*model.Module, error) {
		return &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: name}, nil
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateMenu(ctx, &ast.CreateMenuStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Main_Menu"},
	})
	if err == nil {
		t.Fatal("expected CREATE MENU on an existing menu to fail without OR MODIFY")
	}
}

// TestCreateMenu_OrModifyPreservesIdentity checks that a modify rewrites the
// stored document rather than minting a new one, and does not reset the
// properties MDL does not author. Losing the ID would break every menu widget
// pointing at it; resetting ExportLevel would silently change the module's API.
func TestCreateMenu_OrModifyPreservesIdentity(t *testing.T) {
	existing := &types.MenuDocument{
		ID: "menu-1", ContainerID: "folder-9", Name: "Main_Menu",
		ExportLevel: "Public", Documentation: "kept",
	}
	var updated *types.MenuDocument
	mb := menuBackend(existing)
	mb.GetModuleByNameFunc = func(name string) (*model.Module, error) {
		return &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: name}, nil
	}
	mb.UpdateMenuDocumentFunc = func(md *types.MenuDocument) error { updated = md; return nil }
	mb.CreateMenuDocumentFunc = func(md *types.MenuDocument) error {
		t.Error("OR MODIFY must update the existing menu, not create a second one")
		return nil
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	assertNoError(t, execCreateMenu(ctx, &ast.CreateMenuStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Main_Menu"},
		CreateOrModify: true,
		Items:          []ast.NavMenuItemDef{{Caption: "Home"}},
	}))

	if updated == nil {
		t.Fatal("UpdateMenuDocument was not called")
	}
	if updated.ID != "menu-1" {
		t.Errorf("ID = %q, want the stored menu-1 — a fresh ID orphans every reference", updated.ID)
	}
	if updated.ContainerID != "folder-9" {
		t.Errorf("ContainerID = %q, want folder-9 — a modify must not move the document", updated.ContainerID)
	}
	if updated.ExportLevel != "Public" {
		t.Errorf("ExportLevel = %q, want Public preserved", updated.ExportLevel)
	}
	if updated.Documentation != "kept" {
		t.Errorf("Documentation = %q, want the stored text preserved", updated.Documentation)
	}
	if len(updated.Items) != 1 || updated.Items[0].Caption != "Home" {
		t.Errorf("items were not replaced by the statement's list: %+v", updated.Items)
	}
}

// TestMenuItemsFromAST_Nested covers the AST→model mapping the executor owns,
// including the recursion the fixture's flat Atlas menus cannot exercise.
func TestMenuItemsFromAST_Nested(t *testing.T) {
	page := ast.QualifiedName{Module: "M", Name: "P"}
	mf := ast.QualifiedName{Module: "M", Name: "F"}
	items := menuItemsFromAST([]ast.NavMenuItemDef{{
		Caption: "Top",
		Items: []ast.NavMenuItemDef{
			{Caption: "Pg", Page: &page, Icon: "M.C.i"},
			{Caption: "Mf", Microflow: &mf},
			{Caption: "Plain"},
		},
	}})

	if len(items) != 1 || len(items[0].Items) != 3 {
		t.Fatalf("expected 1 top item with 3 children, got %+v", items)
	}
	kids := items[0].Items
	if kids[0].Page != "M.P" || kids[0].ActionType != "PageAction" {
		t.Errorf("page child = %+v", kids[0])
	}
	if kids[0].IconType != "Forms$IconCollectionIcon" {
		t.Errorf("an ICON clause must record the icon type it round-trips as, got %q", kids[0].IconType)
	}
	if kids[1].Microflow != "M.F" || kids[1].ActionType != "MicroflowAction" {
		t.Errorf("microflow child = %+v", kids[1])
	}
	if kids[2].ActionType != "NoAction" {
		t.Errorf("a target-less item must be NoAction, got %q", kids[2].ActionType)
	}
}
