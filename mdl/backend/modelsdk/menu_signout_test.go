// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	genPages "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// A sign-out MENU ITEM reaches storage through two writers that do not share
// code with the button path: menuActionToGen for a standalone menu document,
// and navMenuAction for the menu inside a navigation profile. Both ended in a
// NoAction default, so both silently produced a dead entry.
//
// Studio Pro stores the same element a button carries — measured on
// ako/TestApp's sign-out menu item: Forms$SignOutClientAction with
// DisabledDuringExecution true and nothing else.

func TestMenuActionToGen_SignOut(t *testing.T) {
	el := menuActionToGen(&types.NavMenuItem{Caption: "Sign out", ActionType: "SignOutAction"})
	g, ok := el.(*genPages.SignOutClientAction)
	if !ok {
		t.Fatalf("got %T, want *pages.SignOutClientAction — a menu item's sign-out fell to NoAction", el)
	}
	if !g.DisabledDuringExecution() {
		t.Error("DisabledDuringExecution is false; the Studio Pro reference stores true")
	}
}

// CONTROL: the two actions that already worked, plus the actionless item, are
// unchanged. A writer that answered SignOut for everything would pass the test
// above.
func TestMenuActionToGen_OtherItemsUnchanged(t *testing.T) {
	cases := []struct {
		name string
		item *types.NavMenuItem
		want string
	}{
		{"page", &types.NavMenuItem{Page: "M.P"}, "Forms$FormAction"},
		{"microflow", &types.NavMenuItem{Microflow: "M.MF"}, "Forms$MicroflowAction"},
		{"plain", &types.NavMenuItem{Caption: "Plain"}, "Forms$NoAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := menuActionToGen(c.item).TypeName(); got != c.want {
				t.Errorf("$Type = %q, want %q", got, c.want)
			}
		})
	}
}

// The navigation-profile writer is the other half, and it builds raw BSON.
func TestNavMenuAction_SignOut(t *testing.T) {
	doc := navMenuAction(types.NavMenuItemSpec{Caption: "Sign out", SignOut: true})
	got := map[string]any{}
	for _, e := range doc {
		got[e.Key] = e.Value
	}
	if got["$Type"] != "Forms$SignOutClientAction" {
		t.Fatalf("$Type = %v, want Forms$SignOutClientAction", got["$Type"])
	}
	if got["DisabledDuringExecution"] != true {
		t.Errorf("DisabledDuringExecution = %v, want true", got["DisabledDuringExecution"])
	}
	if len(doc) != 3 {
		t.Errorf("the action has %d keys, want 3 ($ID, $Type, DisabledDuringExecution): %v", len(doc), doc)
	}
}

// CONTROL: an item with no action still writes Forms$NoAction, so the test
// above proves something about SIGN_OUT rather than about the default going
// away.
func TestNavMenuAction_PlainItemStillNoAction(t *testing.T) {
	doc := navMenuAction(types.NavMenuItemSpec{Caption: "Plain"})
	for _, e := range doc {
		if e.Key == "$Type" && e.Value != "Forms$NoAction" {
			t.Errorf("$Type = %v, want Forms$NoAction", e.Value)
		}
	}
}

// The reader has to produce the name the writers consume, or the round trip
// does not close. That pairing is the actual fix: reading TestApp's item as a
// raw type name would have described it as a plain item.
func TestResolveMenuAction_SignOutUsesTheWriterSName(t *testing.T) {
	item := &types.NavMenuItem{}
	resolveMenuAction(item, genPages.NewSignOutClientAction())
	if item.ActionType != "SignOutAction" {
		t.Errorf("ActionType = %q, want SignOutAction — the writers key on that exact string",
			item.ActionType)
	}
}
