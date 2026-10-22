// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1611 B: `dropdownfilter ddf (defaultValue: 'true')` in a
// Boolean column passed check and exec, then mxbuild failed CE0117 "Error(s) in
// expression." The property is an Expression returning String, and a quoted MDL
// value IS the expression text, so `'true'` stores the expression `true` — a
// Boolean. Each case is one measured value (11.14.0, one filter per value, the
// column type beside it); the controls built at 0 errors.
func TestWidgetExpressionReturnType_DropdownFilterDefault(t *testing.T) {
	for _, tc := range []struct {
		name, attr, value string
		ce0117            bool
	}{
		{"the report: Boolean literal", "IsActive", `'true'`, true},
		{"false", "IsActive", `'false'`, true},
		{"bare name", "Label", `'abc'`, true},
		{"Integer literal", "Label", `'1'`, true},
		{"Integer literal in an Integer column", "Size", `'1'`, true},
		{"Decimal literal", "Label", `'1.5'`, true},
		// controls
		{"the workaround: a String literal", "IsActive", `'''true'''`, false},
		{"String literal", "Label", `'''abc'''`, false},
		{"a String matching no option", "IsActive", `'''tru'''`, false},
		{"empty", "Label", `'empty'`, false},
		{"a String expression", "Label", `'''a'' + ''b'''`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `create page F.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: DATABASE F.Thing) {
    column (Attribute: ` + tc.attr + `) { dropdownfilter ddf (defaultValue: ` + tc.value + `) }
  }
};`
			var hits []string
			for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
				if v.RuleID == "MDL-WIDGET49" {
					hits = append(hits, v.Message+" | "+v.Suggestion)
				}
			}
			if !tc.ce0117 {
				if len(hits) != 0 {
					t.Errorf("want no MDL-WIDGET49, got %q", hits)
				}
				return
			}
			if len(hits) != 1 {
				t.Fatalf("want one MDL-WIDGET49, got %q", hits)
			}
			for _, want := range []string{"defaultValue", "String", "CE0117"} {
				if !strings.Contains(hits[0], want) {
					t.Errorf("want %q in %q", want, hits[0])
				}
			}
		})
	}
}

// The hint gives the spelling that builds: the value as a String literal.
func TestWidgetExpressionReturnType_HintQuotesTheLiteral(t *testing.T) {
	src := `create page F.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: DATABASE F.Thing) {
    column (Attribute: IsActive) { dropdownfilter ddf (defaultValue: 'true') }
  }
};`
	for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
		if v.RuleID == "MDL-WIDGET49" && !strings.Contains(v.Suggestion, `defaultValue: '''true'''`) {
			t.Errorf("hint does not give the String literal: %q", v.Suggestion)
		}
	}
}
