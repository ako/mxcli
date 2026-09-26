// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// TestExtractAliasMap_PathJoinAliasCase guards #652: an association-path join
// whose alias is introduced by an uppercase `AS` resolved to nothing, because
// the path was recovered from the match with a case-sensitive trim of "as".
// The alias then had no entity, every column from it went without type
// inference, and a pass-through length mismatch (CE6770) passed `check`.
// DESCRIBE of a Studio Pro model prints `AS` in upper case, so this is the
// ordinary shape of round-tripped OQL.
func TestExtractAliasMap_PathJoinAliasCase(t *testing.T) {
	cases := []struct {
		name, oql, alias, want string
	}{
		{"lower as", `select r.Name from M.Sale as s join s/M.Sale_UserRole/System.UserRole as r`, "r", "System.UserRole"},
		{"upper AS", `select r.Name from M.Sale as s join s/M.Sale_UserRole/System.UserRole AS r`, "r", "System.UserRole"},
		{"mixed As", `select r.Name from M.Sale as s left join s/M.Sale_UserRole/System.UserRole As r`, "r", "System.UserRole"},
		{"no as", `select r.Name from M.Sale as s join s/M.Sale_UserRole/System.UserRole r`, "r", "System.UserRole"},
		{"quoted end, upper AS", `select o.Id from M.Line AS l JOIN l/M.Line_Order/M."Order" AS o`, "o", "M.Order"},
		{"multi-hop, upper AS", `select c.Name from M.Line AS l JOIN l/M.Line_Order/M.Order/M.Order_Customer/M.Customer AS c`, "c", "M.Customer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAliasMap(tc.oql)
			if got[tc.alias] != tc.want {
				t.Errorf("alias %q resolved to %q, want %q (map: %v)", tc.alias, got[tc.alias], tc.want, got)
			}
		})
	}
}
