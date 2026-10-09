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
)

// mendixlabs/mxcli#1343: describe navigation wrote a menu caption holding an
// apostrophe unescaped — `menu item 'Customer's orders'` — so its output did
// not re-parse ("extraneous input 's' expecting {MENU_KW, '}'}") and a
// describe -> exec round trip lost the navigation statement. Both shapes a
// caption is written in, a sub-menu and an item, must read back unchanged.
func TestDescribeNavigation_CaptionWithApostropheRoundTrips(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	items := []*types.NavMenuItem{{
		Caption: "Customer's menu",
		Items:   []*types.NavMenuItem{{Caption: "Customer's orders", Page: "M.Orders"}},
	}}
	var buf bytes.Buffer
	printMenuMDL(ctx, &buf, items, 1, "CREATE NAVIGATION")
	out := buf.String()

	prog, errs := visitor.Build("create or modify navigation Responsive {\n" + out + "};")
	if len(errs) > 0 {
		t.Fatalf("describe output does not re-parse: %v\n%s", errs[0], out)
	}
	menu := prog.Statements[0].(*ast.AlterNavigationStmt).MenuItems
	if len(menu) != 1 || menu[0].Caption != "Customer's menu" {
		t.Fatalf("sub-menu re-parsed as %+v\n%s", menu, out)
	}
	if len(menu[0].Items) != 1 || menu[0].Items[0].Caption != "Customer's orders" {
		t.Fatalf("item re-parsed as %+v\n%s", menu[0].Items, out)
	}
}

// The notes that name a menu item quote its caption the way the item line
// does, so a note can be matched to its item by the same text.
func TestDescribeNavigation_ActionNoteQuotesCaption(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	var buf bytes.Buffer
	printMenuMDL(ctx, &buf, []*types.NavMenuItem{{Caption: "Customer's orders",
		StoredAction: []byte("x"), ActionType: "Forms$Unknown"}}, 1, "CREATE NAVIGATION")
	if !strings.Contains(buf.String(), "-- menu item 'Customer''s orders':") {
		t.Errorf("note does not quote the caption as the item line does:\n%s", buf.String())
	}
}
