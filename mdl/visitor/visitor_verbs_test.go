// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R6 (ako/mxcli#755): every old verb builds exactly the statement its
// canonical spelling builds, records its code, and the canonical spelling
// records nothing. The registry test covers one example per code; these cover
// every grammar alternative the alias appears in.
func TestR6OldVerbsAreAliases(t *testing.T) {
	cases := []struct {
		old, canonical, code string
	}{
		{"show page M.P;", "describe page M.P;", deprecation.ShowSingleThing},
		{"show project security;", "describe app security;", deprecation.ShowSingleThing},
		{"list project security;", "describe app security;", deprecation.ShowSingleThing},
		{"show security matrix;", "describe security matrix;", deprecation.ShowSingleThing},
		{"show security matrix in M;", "describe security matrix in M;", deprecation.ShowSingleThing},
		{"show structure;", "describe structure;", deprecation.ShowSingleThing},
		{"show structure depth 3 in M all;", "describe structure depth 3 in M all;", deprecation.ShowSingleThing},
		{"show context of M.F;", "describe context of M.F;", deprecation.ShowSingleThing},
		{"show context of M.F depth 4;", "describe context of M.F depth 4;", deprecation.ShowSingleThing},
		{"alter user role Clerk remove module roles (M.User, M.Admin);",
			"alter user role Clerk drop module roles (M.User, M.Admin);", deprecation.UserRoleRemove},
		{"alter settings language remove 'ar_SD';", "alter settings language drop 'ar_SD';", deprecation.SettingsRemove},
		{"alter settings workflows remove group 'Approvers';",
			"alter settings workflows drop group 'Approvers';", deprecation.SettingsRemove},
		{"alter entity M.E add column Note: String(200);", "alter entity M.E add attribute Note: String(200);",
			deprecation.ColumnForAttribute},
		{"alter entity M.E rename column Note to Remark;", "alter entity M.E rename attribute Note to Remark;",
			deprecation.ColumnForAttribute},
		{"alter entity M.E modify column Note: String(400);", "alter entity M.E modify attribute Note: String(400);",
			deprecation.ColumnForAttribute},
		{"alter entity M.E drop column if exists Note;", "alter entity M.E drop attribute if exists Note;",
			deprecation.ColumnForAttribute},
		{"create microflow M.F () begin rest call post 'https://x.org' (Body: template '{}', Timeout: 30) returns nothing; end;",
			"create microflow M.F () begin call rest service post 'https://x.org' (Body: template '{}', Timeout: 30) returns nothing; end;",
			deprecation.RestCall},
		{"describe widget combobox;", "describe widget type combobox;", deprecation.DescribeWidgetType},
		{"describe widget 'com.mendix.widget.web.combobox.Combobox';",
			"describe widget type 'com.mendix.widget.web.combobox.Combobox';", deprecation.DescribeWidgetType},
		// `list` names a plural (ako/mxcli#755): the collections too.
		{"list image collection;", "list image collections;", deprecation.SingularCollectionList},
		{"show image collection in M;", "list image collections in M;", ""},
		{"LIST ICON COLLECTION IN M;", "LIST ICON COLLECTIONS IN M;", deprecation.SingularCollectionList},
		{"list message definition collection in M;", "list message definition collections in M;",
			deprecation.SingularCollectionList},
		{"define fragment Hdr ($p: datasource) as { dynamictext t (Content: 'x') };",
			"create fragment Hdr ($p: datasource) as { dynamictext t (Content: 'x') };", deprecation.DefineFragment},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			want := []string{c.code}
			if c.code == "" { // `show … collection`: both the verb and the plural
				want = []string{deprecation.Show, deprecation.SingularCollectionList}
			}
			if got := deprecationCodes(old); !reflect.DeepEqual(got, want) {
				t.Errorf("old recorded %v, want %v", got, want)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if len(old.Statements) != 1 || !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements:\n old:   %#v\n canon: %#v",
					old.Statements, canon.Statements)
			}
		})
	}
}

// `show navigation`, `show navigation menu` and `show settings` print a
// table, which is a listing: their canonical verb is `list`, the same
// statement, so they are MDL-DEPR002 with the token swap and `list` on them
// records nothing (ako/mxcli#755). They were MDL-DEPR090, whose canonical
// `describe` prints the definition as MDL instead — not the same output.
func TestR6ShowSummaryTablesAreListed(t *testing.T) {
	cases := []struct{ old, canonical string }{
		{"show navigation;", "list navigation;"},
		{"show navigation menu;", "list navigation menu;"},
		{"show navigation menu Responsive;", "list navigation menu Responsive;"},
		{"show settings;", "list settings;"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.Show}) {
				t.Errorf("old recorded %v, want [%s]", got, deprecation.Show)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements")
			}
		})
	}
}

// `show entity X` / `show association X` print a summary that no mdl 1
// statement prints: `describe` prints the definition as MDL, `list entities` /
// `list associations` the summary columns. So they are not aliases of either.
// They are removed under mdl 1 (MDL-V1-SHOWSUMMARY, a new rejection) and keep
// their summary under mdl 0 with that warning. `list entity X` is the same
// statement and goes with them.
func TestR6ShowSummaryOfOneElementIsRemovedUnderMdl1(t *testing.T) {
	for _, src := range []string{
		"show entity M.E;", "show association M.A;", "list entity M.E;", "list association M.A;",
	} {
		t.Run(src, func(t *testing.T) {
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("mdl 0: %v", errs)
			}
			if len(prog.Deprecations) != 0 {
				t.Errorf("mdl 0 recorded deprecations %+v: it is not an alias of anything", prog.Deprecations)
			}
			if n := countNotes(prog.LanguageNotes, showSummaryRemoved.Code); n != 1 {
				t.Errorf("mdl 0: %d %s note(s), want 1", n, showSummaryRemoved.Code)
			}
			if len(prog.Statements) != 1 {
				t.Errorf("mdl 0: %d statements, want the summary statement", len(prog.Statements))
			}
			_, errs = Build("mdl 1;\n" + src)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "describe") {
				t.Errorf("mdl 1: want one error pointing at describe, got %v", errs)
			}
		})
	}
	// Control: the describe and list forms are untouched under both versions.
	for _, src := range []string{"describe entity M.E;", "describe association M.A;", "list entities in M;"} {
		for _, v := range []string{"", "mdl 1;\n"} {
			prog, errs := Build(v + src)
			if len(errs) > 0 || countNotes(prog.LanguageNotes, showSummaryRemoved.Code) != 0 {
				t.Errorf("%q: errs %v, notes %+v", v+src, errs, prog.LanguageNotes)
			}
		}
	}
}

// R7: `show version|status|connections`, `show catalog status` report the
// session, not the model. They are session commands: refused in an mdl 1
// script, warned under mdl 0 (MDL-V1-SESSION, as `status;` is).
func TestShowSessionStateIsASessionCommand(t *testing.T) {
	for _, src := range []string{
		"show version;", "show status;", "show connections;", "show catalog status;",
		"list version;", "list catalog status;",
	} {
		t.Run(src, func(t *testing.T) {
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("mdl 0: %v", errs)
			}
			if n := countNotes(prog.LanguageNotes, sessionCommandInScript.Code); n != 1 {
				t.Errorf("mdl 0: %d %s note(s), want 1", n, sessionCommandInScript.Code)
			}
			_, errs = Build("mdl 1;\n" + src)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "session command") {
				t.Errorf("mdl 1: want one session-command error, got %v", errs)
			}
		})
	}
	// Control: `list catalog tables` lists the model's catalog, not the session.
	if prog := mustBuild(t, "list catalog tables;"); countNotes(prog.LanguageNotes, sessionCommandInScript.Code) != 0 {
		t.Errorf("list catalog tables reported as a session command")
	}
}

// The words that stay: a widget on a page (`describe styling … widget`,
// `describe fragment … widget`), `drop` as the canonical verb, and a plain
// `describe widget type`.
func TestR6CanonicalFormsRecordNothing(t *testing.T) {
	for _, src := range []string{
		"describe styling on page M.P widget btn1;",
		"describe fragment from page M.P widget btn1;",
		"describe widget type combobox;",
		"alter entity M.E drop default on attribute Note;",
		"create association M.A_B from M.A to M.B type Reference storage column;",
		"list navigation homes;",
		"list navigation;",
		"list navigation menu;",
		"list settings;",
	} {
		t.Run(src, func(t *testing.T) {
			if got := deprecationCodes(mustBuild(t, src)); len(got) != 0 {
				t.Errorf("recorded %v, want none", got)
			}
		})
	}
}

// `show project version` is not a statement — `show project` continues only
// with `security` (`show version` is the version statement). The parser
// recovers by dropping the stray word, leaving a showStatement with PROJECT and
// no SECURITY token, and the MDL-DEPR090 listener dereferenced the missing
// token: `mxcli -c "show project version"` died with a nil-pointer panic
// instead of a syntax error. Every recovered `show project …` form must come
// back as an error.
func TestShowProjectWithoutSecurityIsASyntaxErrorNotAPanic(t *testing.T) {
	for _, src := range []string{
		"show project version;",
		"list project version;",
		"show project;",
		"show project roles;",
		"show project security matrix;",
	} {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Build(%q) panicked: %v", src, r)
				}
			}()
			_, errs := Build(src)
			if len(errs) == 0 {
				t.Fatalf("Build(%q) reported no error", src)
			}
			// The hint names the statement that does exist, not the generic
			// "quote the keyword" advice `version` would otherwise attract.
			if src == "show project version;" && !strings.Contains(errs[0].Error(), "`show version`") {
				t.Errorf("Build(%q) error does not point at `show version`: %v", src, errs[0])
			}
		})
	}
}
