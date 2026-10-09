// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1341: "CREATE OR REPLACE NAVIGATION keeps a page item's
// action on a same-caption sub-menu (mx check CE0548; v0.25.0 regression)" —
// CE0548 "Items with subitems cannot have an action themselves". The rewrite
// paired the new sub-menu 'Work' with the stored page item 'Work' by caption
// and carried its Forms$FormAction, because a sub-menu "states no action".
// A sub-menu cannot have one, so nothing stored may be kept on it.

// storedWorkPageItem is the menu as v0.25.0 read it back: a top-level page
// item 'Work' (a Forms$FormAction, which describe prints) beside 'Reports',
// whose action describe cannot print.
func storedWorkPageItem(pageAction, unknown []byte) []*types.NavMenuItem {
	return []*types.NavMenuItem{
		{Caption: "Work", ActionType: "PageAction", Page: "Administration.Account_Overview",
			StoredAction: pageAction, ActionDoc: map[string]any{"$Type": "Forms$FormAction"}},
		{Caption: "Reports", ActionType: "Forms$UnknownFutureClientAction",
			StoredAction: unknown, ActionDoc: map[string]any{"$Type": "Forms$UnknownFutureClientAction"}},
	}
}

func TestNavigationRewrite_SubMenuDoesNotKeepAStoredAction(t *testing.T) {
	pageAction := []byte("stored-form-action")
	unknown := []byte("stored-unknown-action")
	var got types.NavigationProfileSpec
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
				Name: "Responsive", Kind: "Responsive", MenuItems: storedWorkPageItem(pageAction, unknown),
			}}}, nil
		},
		UpdateNavigationProfileFunc: func(_ model.ID, _ string, spec types.NavigationProfileSpec) error {
			got = spec
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	prog, errs := visitor.Build(`create or replace navigation Responsive
  menu (
    menu 'Work' (
      menu item 'Accounts' page Administration.Account_Overview;
      menu item 'Sessions' page Administration.ActiveSessions;
    );
    menu item 'Reports';
  )
;`)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	assertNoError(t, execAlterNavigation(ctx, prog.Statements[0].(*ast.AlterNavigationStmt)))

	if len(got.MenuItems) != 2 || got.MenuItems[0].Caption != "Work" {
		t.Fatalf("spec items = %+v, want 'Work' and 'Reports'", got.MenuItems)
	}
	work := got.MenuItems[0]
	if work.KeepAction != nil {
		t.Errorf("sub-menu 'Work' kept the stored page action %q: mx check CE0548 "+
			"\"Items with subitems cannot have an action themselves\"", work.KeepAction)
	}
	if len(work.Items) != 2 {
		t.Errorf("sub-menu 'Work' has %d items, want 2", len(work.Items))
	}
	if strings.Contains(buf.String(), "menu item 'Work'") {
		t.Errorf("no action is kept on 'Work', so none may be reported:\n%s", buf.String())
	}
	// CONTROL: a leaf that states no action still keeps one MDL cannot print
	// (#980). A converter that kept nothing at all would pass the checks above.
	if !bytes.Equal(got.MenuItems[1].KeepAction, unknown) {
		t.Errorf("leaf 'Reports' must keep its stored action, got KeepAction=%q", got.MenuItems[1].KeepAction)
	}
}

// The menu-document writer shares the decision (menuItemsFromAST).
func TestCreateMenu_SubMenuDoesNotKeepAStoredAction(t *testing.T) {
	pageAction := []byte("stored-form-action")
	unknown := []byte("stored-unknown-action")
	existing := &types.MenuDocument{ID: "menu-1", ContainerID: "folder-9", Name: "Main_Menu",
		Items: storedWorkPageItem(pageAction, unknown)}
	var updated *types.MenuDocument
	mb := menuBackend(existing)
	mb.GetModuleByNameFunc = func(name string) (*model.Module, error) {
		return &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: name}, nil
	}
	mb.UpdateMenuDocumentFunc = func(md *types.MenuDocument) error { updated = md; return nil }

	ctx, _ := newMockCtx(t, withBackend(mb))
	assertNoError(t, execCreateMenu(ctx, &ast.CreateMenuStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Main_Menu"},
		CreateOrModify: true,
		Items: []ast.NavMenuItemDef{
			{Caption: "Work", Items: []ast.NavMenuItemDef{{Caption: "Accounts"}}},
			{Caption: "Reports"},
		},
	}))

	if updated == nil || len(updated.Items) != 2 {
		t.Fatalf("UpdateMenuDocument got %+v", updated)
	}
	if work := updated.Items[0]; work.StoredAction != nil || work.ActionType != "NoAction" {
		t.Errorf("sub-menu 'Work' kept ActionType=%q StoredAction=%q: CE0548", work.ActionType, work.StoredAction)
	}
	// CONTROL: the leaf keeps what MDL cannot print.
	if !bytes.Equal(updated.Items[1].StoredAction, unknown) {
		t.Errorf("leaf 'Reports' must keep its stored action, got %q", updated.Items[1].StoredAction)
	}
}
