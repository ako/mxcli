// SPDX-License-Identifier: Apache-2.0

package visitor

import "testing"

// Issue #641 — a bare [%token%] inside an XPath constraint must be quoted so the
// stored constraint passes Studio Pro's mx check (CE0161). Already-quoted tokens
// must be left untouched (no double-quoting).
func TestNormalizeXPathTokens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[DueDate < [%CurrentDateTime%]]", "[DueDate < '[%CurrentDateTime%]']"},
		{"[System.owner = [%CurrentUser%]]", "[System.owner = '[%CurrentUser%]']"},
		// Already quoted — unchanged.
		{"[DueDate < '[%CurrentDateTime%]']", "[DueDate < '[%CurrentDateTime%]']"},
		{"[System.owner = '[%CurrentUser%]']", "[System.owner = '[%CurrentUser%]']"},
		// No token — unchanged.
		{"[Title = 'abc']", "[Title = 'abc']"},
		// Multiple tokens, mixed quoting.
		{"[A < [%CurrentDateTime%] and B = '[%CurrentUser%]']", "[A < '[%CurrentDateTime%]' and B = '[%CurrentUser%]']"},
		// Underscore token name.
		{"[R = [%UserRole_Admin%]]", "[R = '[%UserRole_Admin%]']"},
	}
	for _, c := range cases {
		if got := normalizeXPathTokens(c.in); got != c.want {
			t.Errorf("normalizeXPathTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
