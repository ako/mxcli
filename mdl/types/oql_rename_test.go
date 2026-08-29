// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

func TestRewriteOQLQualifiedName(t *testing.T) {
	for _, tc := range []struct {
		what      string
		oql       string
		old, new  string
		want      string
		wantCount int
	}{
		{
			what: "the reported case — the name sits mid-string, which both engines' walkers miss",
			oql: "select p.Label as Label, sum(p.Amount) as Total\n" +
				"from MyFirstModule.Period as p\ngroup by p.Label",
			old: "MyFirstModule.Period", new: "MyFirstModule.Timeframe",
			want: "select p.Label as Label, sum(p.Amount) as Total\n" +
				"from MyFirstModule.Timeframe as p\ngroup by p.Label",
			wantCount: 1,
		},
		{
			what: "a QUOTED entity name keeps its quotes — that spelling is how a view refers " +
				"to an entity named after an OQL reserved word, and stripping them is CE0174",
			oql: `select y.Label as Label from MyFirstModule."Year" as y`,
			old: "MyFirstModule.Year", new: "MyFirstModule.PlanningYear",
			want:      `select y.Label as Label from MyFirstModule."PlanningYear" as y`,
			wantCount: 1,
		},
		{
			what: "backtick quoting is preserved too",
			oql:  "select y.L from MyFirstModule.`Year` as y",
			old:  "MyFirstModule.Year", new: "MyFirstModule.Y2",
			want:      "select y.L from MyFirstModule.`Y2` as y",
			wantCount: 1,
		},
		{
			what: "a LONGER name starting with the old one is left alone — the single most " +
				"likely way a substring rewrite corrupts a query",
			oql: "select d.X from MyFirstModule.PeriodDetail as d",
			old: "MyFirstModule.Period", new: "MyFirstModule.Timeframe",
			want:      "select d.X from MyFirstModule.PeriodDetail as d",
			wantCount: 0,
		},
		{
			what: "every occurrence is rewritten, including a join",
			oql: "select a.X from MyFirstModule.Period as a " +
				"join MyFirstModule.Period as b on a.Id = b.Id",
			old: "MyFirstModule.Period", new: "MyFirstModule.T",
			want: "select a.X from MyFirstModule.T as a " +
				"join MyFirstModule.T as b on a.Id = b.Id",
			wantCount: 2,
		},
		{
			what: "a different module with the same entity name is not touched",
			oql:  "select x.A from OtherModule.Period as x",
			old:  "MyFirstModule.Period", new: "MyFirstModule.T",
			want: "select x.A from OtherModule.Period as x", wantCount: 0,
		},
		{
			what: "a module rename moves the module half and leaves the entity name",
			oql:  "select p.A from Old.Period as p",
			old:  "Old.Period", new: "New.Period",
			want: "select p.A from New.Period as p", wantCount: 1,
		},
		{
			what: "an unqualified name is not a qualified name — nothing to do, no panic",
			oql:  "select p.A from MyFirstModule.Period as p",
			old:  "Period", new: "Timeframe",
			want: "select p.A from MyFirstModule.Period as p", wantCount: 0,
		},
	} {
		got, n := RewriteOQLQualifiedName(tc.oql, tc.old, tc.new)
		if got != tc.want {
			t.Errorf("%s:\n  got  %q\n  want %q", tc.what, got, tc.want)
		}
		if n != tc.wantCount {
			t.Errorf("%s: count = %d, want %d", tc.what, n, tc.wantCount)
		}
	}
}

func TestRewriteOQLQualifiedName_AliasIsNotAnEntityReference(t *testing.T) {
	// `as p` is a query-local alias, not a model name. An entity that happens to
	// share its spelling must not drag the alias along with it.
	oql := "select Period.X from MyFirstModule.Period as Period"
	got, n := RewriteOQLQualifiedName(oql, "MyFirstModule.Period", "MyFirstModule.T")
	want := "select Period.X from MyFirstModule.T as Period"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

func TestRewriteOQLQualifiedName_ModuleRename(t *testing.T) {
	// execRenameModule calls RenameReferences with a PREFIX pair ("Old." ->
	// "New."), not a qualified name. That shape has no entity half to split, so
	// it took its own path — without which a module rename left every view's OQL
	// on the old module and mxbuild reported CE0174.
	for _, tc := range []struct {
		what      string
		oql       string
		want      string
		wantCount int
	}{
		{
			what:      "the module half moves, the entity half and the alias do not",
			oql:       "select p.Label as Label from Reporting.Period as p group by p.Label",
			want:      "select p.Label as Label from Analytics.Period as p group by p.Label",
			wantCount: 1,
		},
		{
			what:      "every qualified name in the query moves, whatever the entity",
			oql:       "select a.X from Reporting.Period as a join Reporting.Detail as b on a.Id = b.Id",
			want:      "select a.X from Analytics.Period as a join Analytics.Detail as b on a.Id = b.Id",
			wantCount: 2,
		},
		{
			what:      "a quoted entity half keeps its quotes",
			oql:       `select y.L from Reporting."Year" as y`,
			want:      `select y.L from Analytics."Year" as y`,
			wantCount: 1,
		},
		{
			what:      "a different module is untouched",
			oql:       "select x.A from Other.Period as x",
			want:      "select x.A from Other.Period as x",
			wantCount: 0,
		},
	} {
		got, n := RewriteOQLQualifiedName(tc.oql, "Reporting.", "Analytics.")
		if got != tc.want {
			t.Errorf("%s:\n  got  %q\n  want %q", tc.what, got, tc.want)
		}
		if n != tc.wantCount {
			t.Errorf("%s: count = %d, want %d", tc.what, n, tc.wantCount)
		}
	}
}

func TestRewriteOQLQualifiedName_DeclinesWhenAnAliasShadowsTheModule(t *testing.T) {
	// `as Reporting` makes `Reporting.Label` a COLUMN reference, so rewriting the
	// module name there would corrupt the query. Declining leaves the rename
	// reporting nothing for this document — a visible non-event, which is the
	// right failure mode for a case the function cannot resolve on its own.
	oql := "select Reporting.Label from Reporting.Period as Reporting"
	got, n := RewriteOQLQualifiedName(oql, "Reporting.", "Analytics.")
	if n != 0 || got != oql {
		t.Errorf("rewrote a query whose alias shadows the module:\n  got %q (n=%d)", got, n)
	}
}
