// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// The rest of R2 (ako/mxcli#754): menus, property maps and the database
// connection upgrade together — several old spellings in one statement, in
// either letter case, with comments and layout left as written.
func TestUpgrade_R2NavigationMapsAndDatabaseConnection(t *testing.T) {
	src := `create or replace navigation Responsive
  home page M.Home
  menu (
    menu item 'Home' page M.Home icon Atlas_Core.Atlas.home;
    -- administration
    menu 'Admin' icon glyph 57345 (
      menu item 'Users' microflow M.ShowUsers;
      MENU ITEM 'Out' SIGN_OUT;
    );
  );
CREATE MENU M.Side (MENU ITEM 'Plain';);
create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: { $Asset: M.Asset }, Variables: { $n: Integer = 1 }) {
  dynamictext t (Content: 'Hi {1}', ContentParams: [{1} = Name])
  container c (DesignProperties: ['Spacing': ['margin-top': 'Large'], 'Full width': on])
  snippetcall s (Snippet: M.S, Params: {$Asset: $Asset})
};
create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation Ping ( Method: get, Path: '/p', Headers: ('Accept' = 'application/json'), Response: none )
};
create database connection M.Db
  type 'PostgreSQL'
  connection string @M.Url
  username @M.User
  password @M.Pass
begin
  query Customers
    sql $$select id, name from customer where id > {min}$$
    parameter min: Integer default '0'
    parameter name: String null
    returns M.Customer
    map (
      id as CustomerId,
      name as Name
    );
end;
`
	want := `create or modify navigation Responsive
  home page M.Home
  {
    menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )
    -- administration
    menu 'Admin' ( Icon: glyph 57345 ) {
      menu item 'Users' ( OnClick: call microflow M.ShowUsers )
      MENU ITEM 'Out' ( OnClick: SIGN OUT )
    }
  };
CREATE MENU M.Side {MENU ITEM 'Plain'};
create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $Asset: M.Asset ), Variables: ( $n: Integer = 1 )) {
  dynamictext t (Content: 'Hi {1}', ContentParams: ({1} = Name))
  container c (DesignProperties: ('Spacing': ('margin-top': 'Large'), 'Full width': on))
  snippetcall s (Snippet: M.S, Params: (Asset = $Asset))
};
create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation Ping ( Method: get, Path: '/p', Headers: ('Accept': 'application/json'), Response: none )
};
create database connection M.Db (
  Type: 'PostgreSQL',
  ConnectionString: @M.Url,
  Username: @M.User,
  Password: @M.Pass )
{
  query Customers (
    Sql: $$select id, name from customer where id > {min}$$,
    Parameters: ( min: Integer default '0',
    name: String null ),
    Returns: M.Customer,
    Map: (
      CustomerId = id,
      Name = name
    ) )
};
`
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{
		deprecation.MenuChildrenParens: 2, deprecation.MenuItemClauses: 1,
		deprecation.HeaderMapBraces: 1, deprecation.TemplateParamsBrackets: 1,
		deprecation.DesignPropertiesBrackets: 1, deprecation.SnippetCallParamsBraces: 1,
		deprecation.RestHeaderEquals: 1, deprecation.DatabaseConnectionClauses: 1,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
