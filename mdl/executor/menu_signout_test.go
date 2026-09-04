// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A navigation menu can carry a log-out item, and mxcli could neither author one
// nor read one back. MDL's menu item took PAGE or MICROFLOW only, so:
//
//   - authoring: there was no spelling for it at all;
//   - reading:   ako/TestApp's sign-out menu item ("Item 5") came back as a
//     plain `menu item 'Item 5';`, so DESCRIBE -> exec turned a working
//     sign-out into a dead menu entry — silently, with `mx check` clean.
//
// Studio Pro stores it as the same Forms$SignOutClientAction a BUTTON carries
// (measured on that item): DisabledDuringExecution true, and nothing else.

func signOutMenuStmt(t *testing.T, src string) *ast.CreateMenuStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateMenuStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateMenuStmt", prog.Statements[0])
	}
	return stmt
}

// The spelling that did not exist.
func TestMenuItem_SignOutParses(t *testing.T) {
	stmt := signOutMenuStmt(t, `create or modify menu M.Main (
  menu item 'Sign out' sign_out;
);`)
	if len(stmt.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(stmt.Items))
	}
	if !stmt.Items[0].SignOut {
		t.Error("SIGN_OUT did not reach the AST")
	}
}

// SIGN_OUT names no target, so it must not disturb the qualifiedName list that
// PAGE/MICROFLOW and ICON share — an icon after it still has to land as an icon.
func TestMenuItem_SignOutWithAnIcon(t *testing.T) {
	stmt := signOutMenuStmt(t, `create or modify menu M.Main (
  menu item 'Sign out' sign_out icon Atlas_Core.Atlas.home;
);`)
	item := stmt.Items[0]
	if !item.SignOut {
		t.Error("SignOut lost when an icon follows")
	}
	if item.Icon != "Atlas_Core.Atlas.home" {
		t.Errorf("Icon = %q, want the collection entry — SIGN_OUT must consume no qualifiedName", item.Icon)
	}
}

// The menu-document path: AST -> semantic model.
func TestMenuItemsFromAST_SignOutBecomesAnActionType(t *testing.T) {
	items := menuItemsFromAST([]ast.NavMenuItemDef{
		{Caption: "Sign out", SignOut: true},
		{Caption: "Plain"},
	})
	if items[0].ActionType != "SignOutAction" {
		t.Errorf("ActionType = %q, want SignOutAction", items[0].ActionType)
	}
	// CONTROL: an item with no action is still NoAction. A conversion that
	// stamped every actionless item as sign-out would satisfy the line above.
	if items[1].ActionType != "NoAction" {
		t.Errorf("a plain item became %q", items[1].ActionType)
	}
}

// The navigation-profile path uses a different spec type and its own converter.
func TestConvertMenuItemDef_CarriesSignOut(t *testing.T) {
	spec := convertMenuItemDef(ast.NavMenuItemDef{Caption: "Sign out", SignOut: true})
	if !spec.SignOut {
		t.Error("SignOut did not reach NavMenuItemSpec")
	}
	// CONTROL: the two existing targets are untouched.
	page := ast.QualifiedName{Module: "M", Name: "P"}
	if got := convertMenuItemDef(ast.NavMenuItemDef{Caption: "Home", Page: &page}); got.Page != "M.P" || got.SignOut {
		t.Errorf("a page item came back as %+v", got)
	}
}

// DESCRIBE must emit the spelling exec accepts, or the round trip does not
// close — which is the half that lost TestApp's item.
func TestPrintMenuMDL_RendersSignOut(t *testing.T) {
	var b bytes.Buffer
	printMenuMDL(&b, []*types.NavMenuItem{
		{Caption: "Sign out", ActionType: "SignOutAction"},
		{Caption: "Plain", ActionType: "NoAction"},
	}, 0, "CREATE NAVIGATION")

	out := b.String()
	if !strings.Contains(out, "menu item 'Sign out' sign_out;") {
		t.Errorf("describe output does not round-trip the sign-out item:\n%s", out)
	}
	// CONTROL: a plain item must not gain an action.
	if strings.Contains(out, "'Plain' sign_out") {
		t.Errorf("a plain item was rendered as sign-out:\n%s", out)
	}
}

// The whole point, end to end in one test: describe output must parse back to
// the same thing. A renderer and a parser can each be individually right and
// still not agree.
func TestMenuItem_SignOutRoundTripsThroughDescribe(t *testing.T) {
	var b bytes.Buffer
	printMenuMDL(&b, []*types.NavMenuItem{{Caption: "Sign out", ActionType: "SignOutAction"}},
		0, "CREATE NAVIGATION")

	stmt := signOutMenuStmt(t, "create or modify menu M.Main (\n"+b.String()+");")
	if !stmt.Items[0].SignOut {
		t.Errorf("describe emitted %q, which does not parse back as a sign-out item", b.String())
	}
}
