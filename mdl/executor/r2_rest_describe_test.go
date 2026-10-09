// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The rest of R2 (ako/mxcli#754): describe prints navigation and menus with
// { } children, the property maps in ( ), and the database connection as a
// property list with query children — so its output re-parses without
// recording any deprecated spelling (MDL-DEPR120..127), and builds what was
// described.

func TestDescribeNavigation_MenuItemsAreChildren(t *testing.T) {
	ctx, buf := newMockCtx(t)
	outputNavigationProfile(ctx, &types.NavigationProfile{
		Name:                  "Responsive",
		HomePage:              &types.NavHomePage{Page: "M.Home"},
		ThrowPartialSyncError: true,
		MenuItems: []*types.NavMenuItem{
			{Caption: "Home", Page: "M.Home", Icon: "Atlas_Core.Atlas.home", IconType: "Forms$IconCollectionIcon"},
			{Caption: "Admin", IconType: "Forms$GlyphIcon", IconCode: 57345, Items: []*types.NavMenuItem{
				{Caption: "Users", Microflow: "M.ShowUsers"},
				{Caption: "Out", ActionType: "SignOutAction"},
			}},
		},
	})
	out := buf.String()
	assertCanonicalDescribe(t, out,
		"{\n  menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )",
		"menu 'Admin' ( Icon: glyph 57345 ) {",
		"menu item 'Users' ( OnClick: call microflow M.ShowUsers )",
		"menu item 'Out' ( OnClick: sign out )",
		"};")
	stmt := findStmt[*ast.AlterNavigationStmt](t, reparse(t, out), out)
	if len(stmt.MenuItems) != 2 || len(stmt.MenuItems[1].Items) != 2 || !stmt.MenuItems[1].Items[1].SignOut ||
		stmt.MenuItems[1].IconCode != 57345 || stmt.MenuItems[0].Page == nil {
		t.Errorf("menu did not round-trip: %+v", stmt.MenuItems)
	}
}

// Control: a profile without menu items still ends in `;` and gains no block.
func TestDescribeNavigation_NoMenuNoBlock(t *testing.T) {
	ctx, buf := newMockCtx(t)
	outputNavigationProfile(ctx, &types.NavigationProfile{Name: "Responsive", HomePage: &types.NavHomePage{Page: "M.Home"}, ThrowPartialSyncError: true})
	out := buf.String()
	assertCanonicalDescribe(t, out, "home page M.Home\n;")
	if strings.Contains(out, "{") {
		t.Errorf("a profile with no menu items gained a block:\n%s", out)
	}
}

func TestDescribeMenu_ReParsesCanonically(t *testing.T) {
	md := &types.MenuDocument{Name: "Main_Menu", Items: []*types.NavMenuItem{
		{Caption: "Home", Page: "M.Home"},
		{Caption: "More", Items: []*types.NavMenuItem{{Caption: "Out", ActionType: "SignOutAction"}}},
	}}
	ctx, buf := newMockCtx(t, withBackend(menuBackend(md)))
	assertNoError(t, describeMenu(ctx, ast.QualifiedName{Module: "Atlas_Core", Name: "Main_Menu"}))
	assertCanonicalDescribe(t, buf.String(), "create or modify menu Atlas_Core.Main_Menu {", "menu 'More' {")
}

func TestDescribeDatabaseConnection_PropertiesAndQueryChildren(t *testing.T) {
	conn := &model.DatabaseConnection{
		Name: "Erp", DatabaseType: "PostgreSQL",
		ConnectionString: "M.DbUrl", UserName: "M.DbUser", Password: "M.DbPass",
		Queries: []*model.DatabaseQuery{
			{
				Name: "GetCustomers",
				SQL:  "select id, name from customer where id > {minId} and name = 'it''s'",
				Parameters: []*model.DatabaseQueryParameter{
					{ParameterName: "minId", DataType: "DataTypes$IntegerType", DefaultValue: "0"},
					{ParameterName: "name", DataType: "DataTypes$StringType", EmptyValueBecomesNull: true},
				},
				TableMappings: []*model.DatabaseTableMapping{{Entity: "M.Customer", Columns: []*model.DatabaseColumnMapping{
					{Attribute: "M.Customer.CustomerId", ColumnName: "id"},
					{Attribute: "M.Customer.Name", ColumnName: "name"},
				}}},
			},
			{Name: "Ping", SQL: "select 1"},
		},
	}
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputDatabaseConnectionMDL(ctx, conn, "M"))
	out := buf.String()
	assertCanonicalDescribe(t, out, "create or modify database connection M.Erp (\n  Type: 'PostgreSQL',",
		"query GetCustomers (", "Map: (", "CustomerId = id", "query Ping (")
	stmt := findStmt[*ast.CreateDatabaseConnectionStmt](t, reparse(t, out), out)
	if stmt.DatabaseType != "PostgreSQL" || stmt.ConnectionString != "M.DbUrl" || !stmt.ConnectionStringIsRef ||
		!stmt.UserNameIsRef || !stmt.PasswordIsRef || len(stmt.Queries) != 2 {
		t.Fatalf("connection did not round-trip: %+v", stmt)
	}
	q := stmt.Queries[0]
	if q.SQL != conn.Queries[0].SQL || q.Returns.String() != "M.Customer" || len(q.Parameters) != 2 ||
		q.Parameters[0].DefaultValue != "0" || !q.Parameters[1].TestWithNull ||
		len(q.Mappings) != 2 || q.Mappings[0].ColumnName != "id" || q.Mappings[0].AttributeName != "CustomerId" {
		t.Errorf("query did not round-trip: %+v", q)
	}
}

// Control: a connection without queries ends at its property list.
func TestDescribeDatabaseConnection_NoQueries(t *testing.T) {
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputDatabaseConnectionMDL(ctx, &model.DatabaseConnection{
		Name: "Erp", DatabaseType: "MSSQL", ConnectionString: "M.U", UserName: "M.N", Password: "M.P"}, "M"))
	assertCanonicalDescribe(t, buf.String(), "Password: @M.P\n);")
}

func TestDescribeRestHeaders_AreAMap(t *testing.T) {
	svc := &model.ConsumedRestService{
		Name: "Api", BaseUrl: "https://api.example.com",
		Operations: []*model.RestClientOperation{{Name: "Ping", HttpMethod: "GET", Path: "/ping", ResponseType: "NONE",
			Headers: []*model.RestClientHeader{{Name: "Accept", Value: "application/json"}}}},
	}
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputConsumedRestServiceMDL(ctx, svc, "M"))
	assertCanonicalDescribe(t, buf.String(), "Headers: ('Accept': 'application/json')")
}

// A widget's design properties and text-template parameters are maps, in ( ).
func TestDescribeWidgetMaps_InParens(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$DynamicText", Name: "t", Content: "Hi {1}", Parameters: []string{"Name"},
		DesignProperties: []rawDesignProp{
			{Key: "Spacing", ValueType: "compound", Nested: []rawDesignProp{{Key: "margin-top", ValueType: "option", Option: "Large"}}},
			{Key: "Full width", ValueType: "toggle"},
		},
	}, 1)
	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$ActionButton", Name: "b", Caption: "Go {1}", Parameters: []string{"Title"}, Action: "save changes",
	}, 1)
	out := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + buf.String() + "};\n"
	assertCanonicalDescribe(t, out,
		"ContentParams: ({1} = Name)", "CaptionParams: ({1} = Title)",
		"DesignProperties: ('Spacing': ('margin-top': 'Large'), 'Full width': on)")
}

// A page's header maps — Params and Variables — are in ( ).
func TestDescribePageHeaderMaps_InParens(t *testing.T) {
	mod := mkModule("M")
	h := mkHierarchy(mod)
	page := &pages.Page{
		BaseElement: model.BaseElement{ID: nextID("pg")},
		ContainerID: mod.ID,
		Name:        "Edit",
		Parameters:  []*pages.PageParameter{{Name: "Order", EntityName: "M.Order"}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{page}, nil },
		GetRawUnitFunc: func(model.ID) (map[string]any, error) {
			return map[string]any{"Variables": []any{int32(3), map[string]any{
				"Name": "show", "DefaultValue": "true",
				"VariableType": map[string]any{"$Type": "DataTypes$BooleanType"},
			}}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describePage(ctx, ast.QualifiedName{Module: "M", Name: "Edit"}))
	assertCanonicalDescribe(t, buf.String(), "Params: ( $Order: M.Order )", "Variables: ( $show: Boolean = true )")
}
