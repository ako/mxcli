// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// menuItemRaw builds the minimum a Menus$MenuItem needs to parse: a caption with
// a real translation (parseNavMenuItem deliberately returns nil for an item with
// no caption, no page and no children) plus the action under test.
func menuItemRaw(actionType string) map[string]any {
	return map[string]any{
		"Caption": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{
				int32(3),
				map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Sign out"},
			},
		},
		"Action": map[string]any{"$Type": actionType},
	}
}

// The legacy reader is the other half of reading a sign-out MENU ITEM back.
// Before this case it fell to the raw-type-name default, so the item was
// described as a plain `menu item 'x';` and DESCRIBE -> exec turned ako/TestApp's
// working sign-out entry into a dead one — silently, with mx check clean.
func TestParseNavMenuItem_SignOut(t *testing.T) {
	mi := parseNavMenuItem(menuItemRaw("Forms$SignOutClientAction"))
	if mi == nil {
		t.Fatal("parseNavMenuItem returned nil")
	}
	if mi.ActionType != "SignOutAction" {
		t.Errorf("ActionType = %q, want SignOutAction — the writers and DESCRIBE key on that string",
			mi.ActionType)
	}
}

// CONTROL: the action types already read must be unchanged, and an unknown one
// must still fall through to its raw name rather than being absorbed.
func TestParseNavMenuItem_OtherActionsUnchanged(t *testing.T) {
	cases := []struct {
		typeName string
		want     string
	}{
		{"Forms$FormAction", "PageAction"},
		{"Forms$MicroflowAction", "MicroflowAction"},
		{"Forms$NoAction", "NoAction"},
		{"Forms$SomethingElseAction", "Forms$SomethingElseAction"},
	}
	for _, c := range cases {
		mi := parseNavMenuItem(menuItemRaw(c.typeName))
		if mi.ActionType != c.want {
			t.Errorf("%s -> %q, want %q", c.typeName, mi.ActionType, c.want)
		}
	}
}
