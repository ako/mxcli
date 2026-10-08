// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#877: mxcli-rest wrote `drop microflow if exists X;` between a
// flow's doc comment and its `create`. The comment attached to the drop, which
// ignores it, and six flows lost their documentation with no diagnostic.
func TestDetachedDocComments(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []ast.DetachedDocComment
	}{{
		name: "a drop between the comment and its create",
		src: "/** Fetches the orders. */\n" +
			"drop microflow if exists Rest.GetOrders;\n" +
			"create microflow Rest.GetOrders ($Id: Integer)\nbegin\nend;\n",
		want: []ast.DetachedDocComment{{Line: 1, Statement: "drop microflow if exists Rest.GetOrders",
			Next: "create microflow Rest.GetOrders", NextLine: 3}},
	}, {
		name: "grant, revoke and set take none",
		src: "/** a */\ngrant execute on microflow M.F to M.User;\n" +
			"/** b */\nrevoke execute on microflow M.F from M.User;\n",
		want: []ast.DetachedDocComment{
			{Line: 1, Statement: "grant execute on microflow M.F to M.User"},
			{Line: 3, Statement: "revoke execute on microflow M.F from M.User"},
		},
	}, {
		name: "control: a comment on the create it documents",
		src: "drop microflow if exists Rest.GetOrders;\n/** Fetches the orders. */\n" +
			"create microflow Rest.GetOrders ($Id: Integer)\nbegin\nend;\n",
	}, {
		name: "control: an entity and a constant store theirs",
		src:  "/** e */\ncreate persistent entity M.E (Name: String(20));\n/** c */\ncreate constant M.C (Type: String, DefaultValue: 'x');\n",
	}, {
		name: "a create that stores no documentation",
		src:  "/** m */\ncreate module role Sales.User;\n/** e */\ncreate persistent entity Sales.E (Name: String(20));\n",
		want: []ast.DetachedDocComment{{Line: 1, Statement: "create module role Sales.User",
			Next: "create persistent entity Sales.E", NextLine: 4}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			got := prog.DetachedDocComments
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
