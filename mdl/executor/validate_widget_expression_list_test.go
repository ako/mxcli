// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#750 proposes `dynamicclasses: [ if … then 'a' else 'b' ]` as
// the first-class spelling of an expression property. That text already parses
// — as `propertyValueV3`'s ARRAY alternative — so the value reaches the AST as a
// []string. Both readers take only a string (WidgetV3.GetDynamicClasses via
// GetStringProp, and the column builder's `columnClass` via `v.(string)`), so
// `check` passed, `exec` reported success and the widget was written with no
// dynamic class at all: #999's silent drop, reached by the spelling an issue
// proposes. An empty `[]` is already MDL-WIDGET27; this is the non-empty case.
func TestMDLWIDGET32_ExpressionPropertyWrittenAsList(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			// The shape #750 proposes, verbatim.
			name: "issue shape: dynamicclasses in brackets",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (dynamicclasses: [ if $currentObject/Featured then 'is-featured' else '' ]) { }
}`,
			want: 1,
		},
		{
			name: "canonical casing",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (DynamicClasses: ['a']) { }
}`,
			want: 1,
		},
		{
			name: "datagrid column DynamicCellClass in brackets",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    column c1 (attribute: Name, caption: 'N', DynamicCellClass: [ if $currentObject/Featured then 'hot' else '' ])
  }
}`,
			want: 1,
		},
		{
			// The quoted expression is how the property is written today, and
			// is what reaches storage: the control.
			name: "control: quoted expression",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (dynamicclasses: 'if $currentObject/Featured then ''is-featured'' else ''''') { }
}`,
			want: 0,
		},
		{
			name: "control: quoted column expression",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    column c1 (attribute: Name, caption: 'N', DynamicCellClass: 'if $currentObject/Featured then ''hot'' else ''''')
  }
}`,
			want: 0,
		},
		{
			// Brackets are how MDL spells OTHER properties; the rule is keyed on
			// the property, never on the brackets.
			name: "control: visible takes a bracketed condition",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (visible: [Active = true]) { }
}`,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := widgetViolations(t, tc.src, "MDL-WIDGET32")
			if len(got) != tc.want {
				t.Fatalf("MDL-WIDGET32: got %d violation(s), want %d: %#v", len(got), tc.want, got)
			}
			if tc.want == 0 {
				return
			}
			// The message has to carry its own remedy: the quoted spelling that
			// does reach storage.
			for _, s := range []string{"discarded", "quoted"} {
				if !strings.Contains(got[0].Message+got[0].Suggestion, s) {
					t.Errorf("message should mention %q: %s / %s", s, got[0].Message, got[0].Suggestion)
				}
			}
		})
	}
}
