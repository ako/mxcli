// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ALTER STYLING reported a design-property key the theme RENAMED as if it were
// unknown: "no widget type in this project's theme declares — mxbuild reports
// this as CE6083", and suggested a near-miss by spelling. The key is declared —
// as an old name — and mxbuild answers a stored old name with CE6087 "Design
// properties have been renamed in your theme and need to be updated" (measured
// on Mendix 11.13.0 with Atlas Core 4.1.3's "Align content" / "Spacing bottom",
// PR #679). The authoring paths already say "was renamed to X"; this is the
// same answer on the one statement that writes design properties by name.
func TestAlterStyling_RenamedKey_NamesTheCurrentProperty(t *testing.T) {
	reg := renamedThemeRegistry(t)
	cases := []struct {
		name, set string
		want      []string // in message or suggestion
		wantNot   []string
	}{
		{
			name:    "renamed property, value mapped through old option names",
			set:     `set 'Align content' = 'Left align as column'`,
			want:    []string{"renamed", "Align content (deprecated)", "CE6087", `set 'Align content (deprecated)' = 'Left align as a column'`},
			wantNot: []string{"CE6083"},
		},
		{
			// A Spacing side became one side of a compound, which ALTER STYLING
			// cannot write (one flat value — the MDL-WIDGET12 limit): point at
			// the inline form instead.
			name:    "spacing side renamed into a compound",
			set:     `set 'Spacing bottom' = 'Outer medium'`,
			want:    []string{"renamed", "CE6087", "'Spacing': ['margin-bottom': 'M']", "DesignProperties"},
			wantNot: []string{"CE6083"},
		},
		{
			name:    "multi-select toggle renamed into an option",
			set:     `set 'Hide on phone' = on`,
			want:    []string{"renamed", "CE6087", "'Hide on': ['Phone': on]", "DesignProperties"},
			wantNot: []string{"CE6083"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := parseMDL(t, "alter styling on page M.P widget c1 "+tc.set+";")
			got := validateAlterStylingDesignProps(prog, reg)
			if len(got) != 1 || got[0].RuleID != "MDL-WIDGET11" {
				t.Fatalf("want one MDL-WIDGET11, got %#v", got)
			}
			text := got[0].Message + " | " + got[0].Suggestion
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("lacks %q:\n%s", w, text)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(text, w) {
					t.Errorf("must not say %q for a renamed key:\n%s", w, text)
				}
			}
		})
	}
}

// CONTROL: a key the theme knows under no name is still undeclared, CE6083.
func TestAlterStyling_UnknownKeyStillCE6083(t *testing.T) {
	prog := parseMDL(t, "alter styling on page M.P widget c1 set 'Spacing bottomx' = 'Outer medium';")
	got := validateAlterStylingDesignProps(prog, renamedThemeRegistry(t))
	if len(got) != 1 || !strings.Contains(got[0].Message, "CE6083") || strings.Contains(got[0].Message, "renamed") {
		t.Fatalf("an unknown key must still be reported as undeclared (CE6083); got %#v", got)
	}
}
