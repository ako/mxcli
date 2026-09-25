// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#653: the stored OQL of a view entity is the query, not its
// position in the script. Line 1 always lost its indentation (the source text
// starts at the first token) while lines 2…n kept all of theirs, so the
// indentation describe adds came back on every cycle.
func TestViewEntityOQL_StoresQueryWithoutScriptIndentation(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "query on its own lines",
			src:  "create view entity Shop.V (Total: Integer) as (\n  select\n    sum(s.Amount) as Total\n  from Shop.Sale as s\n);",
			want: "select\n  sum(s.Amount) as Total\nfrom Shop.Sale as s",
		},
		{
			name: "query starts on the as ( line",
			src:  "create view entity Shop.V (Total: Integer) as (select sum(s.Amount) as Total\n    from Shop.Sale as s\n    where s.Amount > 0);",
			want: "select sum(s.Amount) as Total\nfrom Shop.Sale as s\nwhere s.Amount > 0",
		},
		{
			name: "continuation less indented than line 1 bounds the strip",
			src:  "create view entity Shop.V (Total: Integer) as (\n    select sum(s.Amount) as Total\n  from Shop.Sale as s\n);",
			want: "select sum(s.Amount) as Total\nfrom Shop.Sale as s",
		},
		{
			name: "blank line and comment survive",
			src:  "create view entity Shop.V (Total: Integer) as (\n  select sum(s.Amount) as Total\n\n  -- every sale\n  from Shop.Sale as s\n);",
			want: "select sum(s.Amount) as Total\n\n-- every sale\nfrom Shop.Sale as s",
		},
		{
			name: "unindented query is untouched",
			src:  "create view entity Shop.V (Total: Integer) as (\nselect sum(s.Amount) as Total\n  from Shop.Sale as s\n);",
			want: "select sum(s.Amount) as Total\n  from Shop.Sale as s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse failed: %v", errs)
			}
			stmt, ok := prog.Statements[0].(*ast.CreateViewEntityStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.CreateViewEntityStmt", prog.Statements[0])
			}
			if stmt.Query.RawQuery != tc.want {
				t.Errorf("stored OQL\nwant %q\n got %q", tc.want, stmt.Query.RawQuery)
			}
		})
	}
}
