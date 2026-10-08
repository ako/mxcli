// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// The rest of R2 (ako/mxcli#754): navigation and menus, property maps, and
// the database connection. As for the integration documents
// (r2_children_test.go), each canonical form builds without a warning under
// both versions, and each old form builds the SAME statement, records its code
// once per statement, and carries a rewrite that reaches the canonical form.
// Every old form below uses exactly one old spelling, so the one code's
// rewrite is the whole upgrade; mixtures are covered by the upgrade tests.
var r2RestCases = []r2Case{
	{
		name: "rest header",
		code: deprecation.RestHeaderEquals,
		old: `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation GetUser ( Method: get, Path: '/u', Headers: ('Accept' = 'application/json', 'X-Key'='k', 'Auth' = 'Bearer x'), Response: none )
  operation Ping ( Method: get, Path: '/ping', Headers: ('Accept' = '*/*'), Response: none )
};`,
		canonical: `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation GetUser ( Method: get, Path: '/u', Headers: ('Accept': 'application/json', 'X-Key':'k', 'Auth': 'Bearer x'), Response: none )
  operation Ping ( Method: get, Path: '/ping', Headers: ('Accept': '*/*'), Response: none )
};`,
	},
	{
		name: "navigation menu block",
		code: deprecation.MenuChildrenParens,
		old: `create or modify navigation Responsive
  home page M.Home
  menu (
    menu item 'Home' ( OnClick: show page M.Home );
    menu 'Admin' (
      menu item 'Users' ( OnClick: call microflow M.ShowUsers );
      menu item 'Plain';
    );
  )
  login page M.Login;`,
		canonical: `create or modify navigation Responsive
  home page M.Home
  {
    menu item 'Home' ( OnClick: show page M.Home )
    menu 'Admin' {
      menu item 'Users' ( OnClick: call microflow M.ShowUsers )
      menu item 'Plain'
    }
  }
  login page M.Login;`,
	},
	{
		name: "menu document",
		code: deprecation.MenuChildrenParens,
		old: `create or modify menu M.Main folder 'Menus' (
  menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )
  menu 'More' (menu item 'Out' ( OnClick: sign out ));
);`,
		canonical: `create or modify menu M.Main folder 'Menus' {
  menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )
  menu 'More' {menu item 'Out' ( OnClick: sign out )}
};`,
	},
	{
		name: "menu item clauses",
		code: deprecation.MenuItemClauses,
		old: `create menu M.Main {
  menu item 'Home' page M.Home icon Atlas_Core.Atlas.home
  menu item 'Run' microflow M.Run
  menu item 'Pic' page M.Pic icon image M.Images.pic
  menu item 'Glyph' icon glyph 57345
  MENU ITEM 'Bye' SIGN OUT
  menu 'Admin' { menu item 'Users' page M.Users }
};`,
		canonical: `create menu M.Main {
  menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )
  menu item 'Run' ( OnClick: call microflow M.Run )
  menu item 'Pic' ( OnClick: show page M.Pic, Icon: image M.Images.pic )
  menu item 'Glyph' ( Icon: glyph 57345 )
  MENU ITEM 'Bye' ( OnClick: SIGN OUT )
  menu 'Admin' { menu item 'Users' ( OnClick: show page M.Users ) }
};`,
	},
	{
		name: "page header maps",
		code: deprecation.HeaderMapBraces,
		old: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default,
  Params: { $Order: M.Order, $Qty: Integer },
  Variables: { $show: Boolean = true }) { };`,
		canonical: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default,
  Params: ( $Order: M.Order, $Qty: Integer ),
  Variables: ( $show: Boolean = true, )) { };`,
	},
	{
		name:      "snippet header maps",
		code:      deprecation.HeaderMapBraces,
		old:       `create snippet M.S (Params: { $Customer: M.Customer }, Variables: {$x: Integer = 1}) { };`,
		canonical: `create snippet M.S (Params: ( $Customer: M.Customer ), Variables: ($x: Integer = 1)) { };`,
	},
	{
		name: "template parameters",
		code: deprecation.TemplateParamsBrackets,
		old: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1} of {2}', ContentParams: [{1} = 'a', {2} = Name format (decimalPrecision: 2)])
  actionbutton b (Caption: 'Go {1}', CaptionParams: [{1} = Title], Action: save changes)
  image i (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: [{1} = PictureUrl])
};`,
		canonical: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1} of {2}', ContentParams: ({1} = 'a', {2} = Name format (decimalPrecision: 2)))
  actionbutton b (Caption: 'Go {1}', CaptionParams: ({1} = Title,), Action: save changes)
  image i (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: ({1} = PictureUrl))
};`,
	},
	{
		name: "design properties",
		code: deprecation.DesignPropertiesBrackets,
		old: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  container c (DesignProperties: ['Spacing': ['margin-top': 'Large', 'margin-bottom': 'Medium'], 'Full width': on]) {
    container d (DesignProperties: [])
  }
};`,
		canonical: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  container c (DesignProperties: ('Spacing': ('margin-top': 'Large', 'margin-bottom': 'Medium'), 'Full width': on,)) {
    container d (DesignProperties: ())
  }
};`,
	},
	{
		name: "snippet call arguments",
		code: deprecation.SnippetCallParamsBraces,
		old: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Asset: M.Asset)) {
  snippetcall s (Snippet: M.S, Params: {$Asset: $Asset, Agent:$Asset})
};`,
		canonical: `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Asset: M.Asset)) {
  snippetcall s (Snippet: M.S, Params: (Asset = $Asset, Agent = $Asset))
};`,
	},
	{
		name: "database connection",
		code: deprecation.DatabaseConnectionClauses,
		old: `create or modify database connection M.Erp folder 'Db'
  type 'PostgreSQL'
  connection string @M.DbUrl
  host 'db.local'
  port 5432
  database 'erp'
  username @M.DbUser
  password 'secret'
begin
  query GetCustomers
    sql $$select id, name from customer where id > {minId} and name = {name}$$
    parameter minId: Integer default '0'
    parameter name: String null
    returns M.Customer
    map (
      id as CustomerId,
      name as Name
    );
  query Ping sql 'select 1';
end;`,
		canonical: `create or modify database connection M.Erp folder 'Db' (
  Type: 'PostgreSQL',
  ConnectionString: @M.DbUrl,
  Host: 'db.local',
  Port: 5432,
  DatabaseName: 'erp',
  Username: @M.DbUser,
  Password: 'secret',
) {
  query GetCustomers (
    Sql: $$select id, name from customer where id > {minId} and name = {name}$$,
    Parameters: ( minId: Integer default '0', name: String null ),
    Returns: M.Customer,
    Map: ( CustomerId = id, Name = name ),
  )
  query Ping ( Sql: 'select 1' )
};`,
	},
}

func TestR2Rest_CanonicalFormBuildsWithoutWarning(t *testing.T) {
	for _, c := range r2RestCases {
		for _, header := range []string{"", "mdl 1;\n"} {
			prog := buildNoErrors(t, header+c.canonical)
			if len(prog.Statements) != 1 {
				t.Errorf("%s, header %q: %d statements", c.name, header, len(prog.Statements))
			}
			if len(prog.Deprecations) != 0 {
				t.Errorf("%s, header %q: the canonical form recorded %+v", c.name, header, prog.Deprecations)
			}
		}
	}
}

func TestR2Rest_OldFormIsADeprecatedAlias(t *testing.T) {
	for _, c := range r2RestCases {
		for _, header := range []string{"", "mdl 1;\n"} {
			old := buildNoErrors(t, header+c.old)
			canon := buildNoErrors(t, header+c.canonical)
			if n := countDeprecations(old, c.code); n != 1 {
				t.Errorf("%s, header %q: recorded %s %d times, want once per statement (%+v)", c.name, header, c.code, n, old.Deprecations)
			}
			for _, d := range old.Deprecations {
				if d.Code != c.code {
					t.Errorf("%s: also recorded %s; each case exercises one spelling", c.name, d.Code)
				}
				if d.Fix == nil || len(d.Fix.Edits) == 0 {
					t.Errorf("%s: %s carries no rewrite", c.name, d.Code)
				}
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("%s, header %q: the two forms build different statements:\nold:   %#v\ncanon: %#v",
					c.name, header, old.Statements[0], canon.Statements[0])
			}
		}
	}
}

// The recorded rewrite, applied to the old form, gives a script that records
// nothing and builds the same statement.
func TestR2Rest_RewriteReachesTheCanonicalForm(t *testing.T) {
	for _, c := range r2RestCases {
		prog := buildNoErrors(t, c.old)
		out := applyAllFixes(prog, c.old)
		again := buildNoErrors(t, out)
		if len(again.Deprecations) != 0 {
			t.Errorf("%s: the rewrite still records %+v:\n%s", c.name, again.Deprecations, out)
		}
		if !reflect.DeepEqual(prog.Statements, again.Statements) {
			t.Errorf("%s: the rewrite changed the statement:\n%s", c.name, out)
		}
	}
}

// applyAllFixes applies every recorded rewrite the way mdl/upgrade does: two
// edits at one offset apply in the order they were recorded, an insertion
// before a replacement.
func applyAllFixes(prog *ast.Program, src string) string {
	var edits []ast.TextEdit
	for _, d := range prog.Deprecations {
		if d.Fix != nil {
			edits = append(edits, d.Fix.Edits...)
		}
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Start != edits[j].Start {
			return edits[i].Start < edits[j].Start
		}
		return edits[i].Stop-edits[i].Start < edits[j].Stop-edits[j].Start
	})
	runes := []rune(src)
	var b strings.Builder
	at := 0
	for _, e := range edits {
		b.WriteString(string(runes[at:e.Start]))
		b.WriteString(e.Text)
		at = e.Stop
	}
	b.WriteString(string(runes[at:]))
	return b.String()
}

// The rewrites are exact: each old form upgrades to the canonical text the
// case spells out, except where the case's canonical form deliberately adds a
// trailing comma (which the rewrite has no reason to write).
func TestR2Rest_RewriteText(t *testing.T) {
	for src, want := range map[string]string{
		`create menu M.M (menu item 'A' page M.A icon Atlas_Core.Atlas.home; menu 'B' icon glyph 1 (menu item 'C' sign_out;););`: `create menu M.M {menu item 'A' ( OnClick: show page M.A, Icon: Atlas_Core.Atlas.home ) menu 'B' ( Icon: glyph 1 ) {menu item 'C' ( OnClick: sign out )}};`,
		`create or modify navigation Responsive menu (menu item 'A' microflow M.F;) home page M.H;`:                              `create or modify navigation Responsive {menu item 'A' ( OnClick: call microflow M.F )} home page M.H;`,
		`create page M.P (Title: 'P', Layout: L.L) { snippetcall s (Snippet: M.S, Params: {$A: $A}) };`:                          `create page M.P (Title: 'P', Layout: L.L) { snippetcall s (Snippet: M.S, Params: (A = $A)) };`,
		// Half-converted: the braces are new, the sub-menu's icon clause and the items' `;` old.
		"create menu M.M { menu 'X' icon glyph 1 { menu item 'a' page M.P; }; };":                                        "create menu M.M { menu 'X' ( Icon: glyph 1 ) { menu item 'a' ( OnClick: show page M.P ) } };",
		`create database connection M.Db type 'x' connection string @M.C;`:                                               `create database connection M.Db ( Type: 'x', ConnectionString: @M.C );`,
		"create database connection M.Db type 'x' connection string @M.C begin query Q sql 'select 1' returns M.E; end;": "create database connection M.Db ( Type: 'x', ConnectionString: @M.C ) { query Q ( Sql: 'select 1', Returns: M.E ) };",
	} {
		prog := buildNoErrors(t, src)
		if got := applyAllFixes(prog, src); got != want {
			t.Errorf("rewrite of\n  %s\ngot\n  %s\nwant\n  %s", src, got, want)
		}
	}
}

// The new property lists are new syntax, so a key they do not know is an
// error rather than something warned about and dropped.
func TestR2Rest_DatabaseConnectionRejectsUnknownKeys(t *testing.T) {
	for src, want := range map[string]string{
		`create database connection M.Db (Type: 'x', Server: 'h');`:                                    "unknown property 'Server'",
		`create database connection M.Db (Type: 'x', Port: 'many');`:                                   "Port takes a number",
		`create database connection M.Db (Type: 'x') { query Q ( Sql: 'x', Limit: 'y' ) };`:            "unknown property 'Limit'",
		`create database connection M.Db (Type: 'x') { query Q ( Sql: 'x', Map: ( a: String ) ) };`:    "Map takes a column map",
		`create database connection M.Db (Type: 'x') { query Q ( Sql: 'x', Parameters: ( a = b ) ) };`: "Parameters takes a parameter list",
	} {
		_, errs := Build(src)
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), want) {
			t.Errorf("%q: errors %v, want one containing %q", src, errs, want)
		}
	}
}

// A menu item's OnClick takes only the three actions an item can carry.
func TestR2Rest_MenuItemActionIsRestricted(t *testing.T) {
	if _, errs := Build(`create menu M.M { menu item 'x' ( OnClick: save changes ) };`); len(errs) == 0 {
		t.Error("a menu item accepted `save changes`, which a menu item cannot carry")
	}
}

// The new property lists are strict in the other direction too: a key written
// twice is an error rather than a silent last-wins (a menu item with two
// OnClick actions was written with whichever the builder checked first), a
// query must say its SQL (the old clause form required it; the property list
// made it optional by accident), and a sub-menu carries no OnClick (the old
// form had no way to write one, and describe never prints one).
func TestR2Rest_NewPropertyListsRejectDuplicatesAndOmissions(t *testing.T) {
	for src, want := range map[string]string{
		`create database connection M.Db (Type: 'x', ConnectionString: @M.C, Type: 'y');`:                    "Type is written twice",
		`create database connection M.Db (Type: 'x') { query Q ( Sql: 'x', Sql: 'y' ) };`:                    "Sql is written twice",
		`create database connection M.Db (Type: 'x') { query Q ( Returns: M.E ) };`:                          "query Q on database connection M.Db has no Sql",
		`create menu M.M { menu item 'x' ( OnClick: show page M.P, OnClick: call microflow M.F ) };`:         "OnClick is written twice",
		`create menu M.M { menu item 'x' ( Icon: glyph 57345, Icon: glyph 57346 ) };`:                        "Icon is written twice",
		`create menu M.M { menu 'Sub' ( OnClick: show page M.P ) { menu item 'x' ( OnClick: sign out ) } };`: "a sub-menu has no OnClick",
	} {
		_, errs := Build(src)
		found := false
		for _, e := range errs {
			found = found || strings.Contains(e.Error(), want)
		}
		if !found {
			t.Errorf("%q: errors %v, want one containing %q", src, errs, want)
		}
	}
	// Control: the same shapes written once build cleanly.
	for _, src := range []string{
		`create database connection M.Db (Type: 'x', ConnectionString: @M.C) { query Q ( Sql: 'x', Returns: M.E ) };`,
		`create menu M.M { menu item 'x' ( OnClick: show page M.P, Icon: glyph 57345 ) menu 'Sub' ( Icon: glyph 57345 ) { menu item 'y' } };`,
	} {
		if _, errs := Build(src); len(errs) != 0 {
			t.Errorf("%q: unexpected errors %v", src, errs)
		}
	}
}
