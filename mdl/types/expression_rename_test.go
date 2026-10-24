// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

func TestRewriteExpressionReferences(t *testing.T) {
	cases := []struct {
		name, in, old, new, want string
		n                        int
	}{
		// Constants are always written @Module.Name, so a whole-string match
		// never saw one — not even a constant that is the whole expression
		// (ako/mxcli#1671).
		{"constant alone", `@Shop.C_Root`, "Shop.C_Root", "Shop.C_Base", `@Shop.C_Base`, 1},
		{"constant in an expression", `@Shop.C_Root + '/orders'`, "Shop.C_Root", "Shop.C_Base", `@Shop.C_Base + '/orders'`, 1},
		{"constant twice", `@Shop.C_Root + @Shop.C_Root`, "Shop.C_Root", "Shop.C_Base", `@Shop.C_Base + @Shop.C_Base`, 2},
		{"constant in an XPath", `[Total > @Shop.C_Limit]`, "Shop.C_Limit", "Shop.C_Max", `[Total > @Shop.C_Max]`, 1},
		{"constant after an operator, no spaces", `$Max>@Shop.C_Limit`, "Shop.C_Limit", "Shop.C_Max", `$Max>@Shop.C_Max`, 1},
		{"a longer constant name is another constant", `@Shop.C_RootX + @Shop.C_Root2`, "Shop.C_Root", "Shop.C_Base", `@Shop.C_RootX + @Shop.C_Root2`, 0},
		{"another module's constant of the same name", `@Other.C_Root`, "Shop.C_Root", "Shop.C_Base", `@Other.C_Root`, 0},
		{"a module whose name ends in the old module's", `@BigShop.C_Root`, "Shop.C_Root", "Shop.C_Base", `@BigShop.C_Root`, 0},

		// A string literal is data, not a reference: Mendix quotes with ' and
		// escapes a quote by doubling it.
		{"inside a string literal", `'see @Shop.C_Root'`, "Shop.C_Root", "Shop.C_Base", `'see @Shop.C_Root'`, 0},
		{"after an escaped quote inside a literal", `'it''s @Shop.C_Root' + @Shop.C_Root`, "Shop.C_Root", "Shop.C_Base", `'it''s @Shop.C_Root' + @Shop.C_Base`, 1},
		{"between two literals", `'a' + @Shop.C_Root + 'b'`, "Shop.C_Root", "Shop.C_Base", `'a' + @Shop.C_Base + 'b'`, 1},

		// An enumeration value is Module.Enum.Value; the whole-string prefix
		// match only caught it when it was the entire expression.
		{"enumeration value in a condition", `$S = Shop.Status.Open and $Max > 1`, "Shop.Status", "Shop.State", `$S = Shop.State.Open and $Max > 1`, 1},
		{"enumeration value after a parenthesis", `if($S=Shop.Status.Open)then 1 else 2`, "Shop.Status", "Shop.State", `if($S=Shop.State.Open)then 1 else 2`, 1},
		{"enumeration value in a literal", `'Shop.Status.Open'`, "Shop.Status", "Shop.State", `'Shop.Status.Open'`, 0},
		{"a longer enumeration name", `Shop.StatusCode.A`, "Shop.Status", "Shop.State", `Shop.StatusCode.A`, 0},
		{"the bare name, with no member", `Shop.Status`, "Shop.Status", "Shop.State", `Shop.Status`, 0},
		// An association step in a path is not a member access, even if an
		// association could share the name.
		{"association path step", `$o/Shop.Status/Name`, "Shop.Status", "Shop.State", `$o/Shop.Status/Name`, 0},

		// Only a qualified name is rewritten here: a module rename arrives as a
		// prefix pair, and a bare module name as itself — neither is this
		// function's business.
		{"module prefix pair", `@Shop.C_Root`, "Shop.", "Store.", `@Shop.C_Root`, 0},
		{"bare module name", `@Shop.C_Root`, "Shop", "Store", `@Shop.C_Root`, 0},
		{"no mention at all", `$x + 1`, "Shop.C_Root", "Shop.C_Base", `$x + 1`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, n := RewriteExpressionReferences(tc.in, tc.old, tc.new)
			if got != tc.want || n != tc.n {
				t.Errorf("RewriteExpressionReferences(%q, %q, %q) = %q, %d; want %q, %d",
					tc.in, tc.old, tc.new, got, n, tc.want, tc.n)
			}
		})
	}
}
