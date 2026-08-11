// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
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

	assertContainsStr(t, out, "-- Menu: Atlas_Core.Main_Menu (3 top-level item(s))")
	assertContainsStr(t, out, "read-only")
	assertContainsStr(t, out, "menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;")

	// A sub-menu opens a nested block and its children are indented one level in.
	assertContainsStr(t, out, "menu 'Admin' (")
	assertContainsStr(t, out, "    menu item 'Accounts' page Administration.Account_Overview;")
	assertContainsStr(t, out, "    menu item 'Rebuild' microflow Administration.Rebuild;")

	// The glyph icon is reported rather than dropped, and names no statement that
	// could author it — a menu document cannot be authored at all.
	assertContainsStr(t, out, "is not reproducible by MDL")
	if strings.Contains(out, "CREATE NAVIGATION") {
		t.Errorf("menu output should not point at CREATE NAVIGATION, which cannot author a menu document:\n%s", out)
	}
}

func TestDescribeMenu_Empty(t *testing.T) {
	md := &types.MenuDocument{Name: "Empty_Menu"}
	ctx, buf := newMockCtx(t, withBackend(menuBackend(md)))
	assertNoError(t, describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Empty_Menu"}))
	assertContainsStr(t, buf.String(), "{ }")
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
