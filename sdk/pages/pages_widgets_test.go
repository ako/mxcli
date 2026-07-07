// SPDX-License-Identifier: Apache-2.0

package pages

import "testing"

// TestStaticVisibleExpression locks in the mapping from a static/string `Visible`
// value to a ConditionalVisibilitySettings expression: page widgets have no plain
// boolean Visible field, so `false` → "false", an expression string passes
// through, and the default-visible cases signal "no settings node".
func TestStaticVisibleExpression(t *testing.T) {
	cases := []struct {
		name       string
		in         any
		wantExpr   string
		wantHasSet bool
	}{
		{"bool true → default visible", true, "", false},
		{"bool false → hidden", false, "false", true},
		{"string true → default visible", "true", "", false},
		{"string TRUE (ci) → default visible", "TRUE", "", false},
		{"string false → hidden", "false", "false", true},
		{"empty string → default visible", "", "", false},
		{"expression passes through", "$currentObject/Name != ''", "$currentObject/Name != ''", true},
		{"expression trimmed", "  $currentObject/Active  ", "$currentObject/Active", true},
		{"nil → no setting", nil, "", false},
		{"other type → no setting", 42, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotExpr, gotHas := StaticVisibleExpression(c.in)
			if gotExpr != c.wantExpr || gotHas != c.wantHasSet {
				t.Errorf("StaticVisibleExpression(%#v) = (%q, %v), want (%q, %v)",
					c.in, gotExpr, gotHas, c.wantExpr, c.wantHasSet)
			}
		})
	}
}
