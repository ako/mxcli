// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func navIconWarnings(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL077" {
			out = append(out, v)
		}
	}
	return out
}

// A menu item with no icon is flagged.
//
// Mendix's navigation sidebar collapses to an icon rail. A collapsed item shows
// its icon; an item without one shows the first few characters of its caption,
// which is rarely enough to tell "Orders" from "Order lines" — so the menu is
// unusable in exactly the state most users leave it in.
//
// The icon is optional in the grammar and nothing said anything, so an item
// written without one built cleanly, passed `mx check`, and only looked wrong in
// a browser.
func TestValidateMenuItemIcons_FlagsMissingIcon(t *testing.T) {
	stmt := &ast.AlterNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{
			{Caption: "Home", Icon: "Atlas_Core.Atlas.home", IconKind: types.MenuIconCollection},
			{Caption: "Orders"},
		},
	}

	got := navIconWarnings(validateMenuItemIcons(stmt))
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "Orders") {
		t.Errorf("warning does not name the item: %q", got[0].Message)
	}
	if got[0].Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want warning — a menu without icons builds and runs, "+
			"it is just hard to use collapsed", got[0].Severity)
	}
}

// The control: an item WITH an icon must stay silent, or the rule is "warn on
// every menu item" and the first assertion proves nothing.
func TestValidateMenuItemIcons_IconedItemIsSilent(t *testing.T) {
	stmt := &ast.AlterNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{
			{Caption: "Home", Icon: "Atlas_Core.Atlas.home", IconKind: types.MenuIconCollection},
			{Caption: "Admin", Icon: `Atlas_Core.Atlas."align-center"`, IconKind: types.MenuIconCollection, Items: []ast.NavMenuItemDef{
				{Caption: "Users", Icon: "Atlas_Core.Atlas.user", IconKind: types.MenuIconCollection},
			}},
		},
	}
	if got := navIconWarnings(validateMenuItemIcons(stmt)); len(got) != 0 {
		t.Errorf("a fully iconed menu produced %d warnings: %+v", len(got), got)
	}
}

// Sub-items are menu items too. A submenu's children render in the flyout the
// collapsed rail opens, and the parent itself sits ON the rail.
func TestValidateMenuItemIcons_RecursesIntoSubItems(t *testing.T) {
	stmt := &ast.AlterNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{
			{Caption: "Admin", Icon: "Atlas_Core.Atlas.cog", IconKind: types.MenuIconCollection, Items: []ast.NavMenuItemDef{
				{Caption: "Users"},
				{Caption: "Roles", Icon: "Atlas_Core.Atlas.group", IconKind: types.MenuIconCollection},
				{Caption: "Deep", Items: []ast.NavMenuItemDef{{Caption: "Deeper"}}},
			}},
		},
	}
	got := navIconWarnings(validateMenuItemIcons(stmt))
	if len(got) != 3 {
		t.Fatalf("got %d warnings, want 3 (Users, Deep, Deeper): %+v", len(got), got)
	}
	joined := ""
	for _, v := range got {
		joined += v.Message + "\n"
	}
	for _, want := range []string{"Users", "Deep", "Deeper"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q not reported; recursion stops short:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Roles") {
		t.Errorf("an iconed sub-item was reported:\n%s", joined)
	}
}

// A standalone menu document carries the same items, so it gets the same rule —
// CREATE MENU and CREATE NAVIGATION share NavMenuItemDef precisely so the two
// cannot diverge.
func TestValidateMenuItemIcons_CoversMenuDocuments(t *testing.T) {
	stmt := &ast.CreateMenuStmt{
		Name:  ast.QualifiedName{Module: "MyModule", Name: "Main_Menu"},
		Items: []ast.NavMenuItemDef{{Caption: "Reports"}},
	}
	got := navIconWarnings(validateMenuItemIcons(stmt))
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1 for a menu document: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "MyModule.Main_Menu") {
		t.Errorf("warning does not name the document: %q", got[0].Message)
	}
}

// Nothing to say about a statement with no menu at all, or another statement
// type entirely.
func TestValidateMenuItemIcons_Quiet(t *testing.T) {
	if got := validateMenuItemIcons(&ast.AlterNavigationStmt{ProfileName: "Responsive"}); len(got) != 0 {
		t.Errorf("a profile with no MENU block warned: %+v", got)
	}
	if got := validateMenuItemIcons(&ast.CreateEntityStmt{}); len(got) != 0 {
		t.Errorf("an unrelated statement warned: %+v", got)
	}
}

// The rule needs no project, so it must fire under `mxcli check` with none —
// which is how CI runs it. A rule that only works with -p is inert in CI, the
// trap that hid four defects in the widget work.
func TestValidateMenuItemIcons_RunsWithoutAProject(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.AlterNavigationStmt{
			ProfileName: "Responsive",
			MenuItems:   []ast.NavMenuItemDef{{Caption: "Orders"}},
		},
	}}
	if got := navIconWarnings(ValidateProgram(prog, "")); len(got) != 1 {
		t.Errorf("got %d MDL077 from ValidateProgram with no project, want 1: %+v", len(got), got)
	}
}

// The trap this rule walked straight into: a glyph icon has a numeric code and
// NO name, so a check written as `Icon == ""` reports an item that plainly has
// an icon. Every Studio Pro-authored menu is full of them — the fixture's own
// Home item carries glyph 57377 — so the naive rule warned about the majority of
// real menus while claiming they had no icon.
func TestValidateMenuItemIcons_AGlyphIconIsAnIcon(t *testing.T) {
	stmt := &ast.AlterNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{
			{Caption: "Home", IconKind: types.MenuIconGlyph, IconCode: 57377},
			{Caption: "Logo", IconKind: types.MenuIconImage, Icon: "MyMod.Images.logo"},
		},
	}
	if got := navIconWarnings(validateMenuItemIcons(stmt)); len(got) != 0 {
		t.Errorf("warned about items that have icons: %+v", got)
	}
}
