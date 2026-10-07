// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// pageWidgetsCatalog runs the REAL catalog builder, in full mode (the only mode
// that fills widgets_data), over one page whose stored widget tree has the three
// shapes the structure annotation has to tell apart:
//
//	LayoutGrid                      — not a data widget
//	  └ column → Data grid 2 (Order) — a data widget, but not at the page root
//	DataView (Customer)              — a data widget at the root
//	  └ ListView (Order)             — nested inside another data widget
//
// Seeding widgets_data by hand would only prove the reader agrees with the
// columns the test typed; the reader named a column (ParentWidget) that the
// builder has never written.
func pageWidgetsCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	const mod = model.ID("mod-sales")
	entity := func(name string) map[string]any {
		return map[string]any{
			"$Type":     "Forms$DataViewSource",
			"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": name},
		}
	}
	page := map[string]any{
		"$Type": "Forms$Page",
		"FormCall": map[string]any{
			"$Type": "Forms$LayoutCall",
			"Arguments": []any{int32(2), map[string]any{
				"$Type": "Forms$FormCallArgument",
				"Widgets": []any{int32(2),
					map[string]any{
						"$ID": "w-grid", "$Type": "Forms$LayoutGrid", "Name": "layoutGrid1",
						"Rows": []any{int32(2), map[string]any{
							"$Type": "Forms$LayoutGridRow",
							"Columns": []any{int32(2), map[string]any{
								"$Type": "Forms$LayoutGridColumn",
								"Widgets": []any{int32(2), map[string]any{
									"$ID": "w-dg2", "$Type": "CustomWidgets$CustomWidget", "Name": "dataGrid1",
									"Type":   map[string]any{"$Type": "CustomWidgets$CustomWidgetType", "WidgetId": "com.mendix.widget.web.datagrid.Datagrid"},
									"Object": map[string]any{"$Type": "CustomWidgets$WidgetObject", "Properties": []any{int32(2), entity("Sales.Order")}},
								}},
							}},
						}},
					},
					map[string]any{
						"$ID": "w-dv", "$Type": "Forms$DataView", "Name": "dataView1",
						"DataSource": entity("Sales.Customer"),
						"Widgets": []any{int32(2), map[string]any{
							"$ID": "w-lv", "$Type": "Forms$ListView", "Name": "listView1",
							"DataSource": entity("Sales.Order"),
						}},
					},
				},
			}},
		},
	}
	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "Sales"}}, nil
		},
		ListPagesFunc: func() ([]*pages.Page, error) {
			return []*pages.Page{
				{BaseElement: model.BaseElement{ID: "pg-1"}, ContainerID: mod, Name: "Order_Overview"},
				{BaseElement: model.BaseElement{ID: "pg-2"}, ContainerID: mod, Name: "Home"},
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListRulesFunc:     func() ([]*microflows.Rule, error) { return nil, nil },
		GetNavigationFunc: func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			if id == "pg-1" {
				return page, nil
			}
			return nil, nil
		},
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := catalog.NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}
	return cat
}

// `show structure depth 2|3` annotates each page with its data widgets, e.g.
// `Page Sales.Order_Overview [DataView<Customer>, ...]`. It never did: the query
// filtered on a ParentWidget column that widgets_data does not have, the error
// was discarded, and every page printed bare.
//
// What the annotation lists is the page's OUTERMOST data widgets — the ones
// that establish a data context — so a data grid inside a layout grid counts
// (a layout grid is not a data context) and a list view nested in a data view
// does not (it is a detail of that data view).
func TestStructurePagesAnnotatesOutermostDataWidgets(t *testing.T) {
	cat := pageWidgetsCatalog(t)
	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat

	if err := structurePages(ctx, "Sales"); err != nil {
		t.Fatalf("structurePages: %v", err)
	}
	out := buf.String()
	want := "  Page Sales.Order_Overview [DataView<Customer>, Datagrid<Order>]\n"
	if !strings.Contains(out, want) {
		t.Errorf("page annotation missing or wrong.\nwant line: %q\ngot:\n%s", want, out)
	}
	if !strings.Contains(out, "  Page Sales.Home\n") {
		t.Errorf("a page without data widgets should print bare; got:\n%s", out)
	}
}

// The structure queries are fixed SQL against tables the builder owns, so a
// failing one is a column or table drift inside mxcli — never a property of
// the user's project. Discarding the error is how ParentWidget went unnoticed
// from the initial commit on; it has to reach the caller.
func TestStructurePagesReportsCatalogQueryErrors(t *testing.T) {
	cat := pageWidgetsCatalog(t)
	if _, err := cat.Query("DROP VIEW widgets"); err != nil {
		t.Fatalf("drop view: %v", err)
	}
	ctx, _ := newMockCtx(t)
	ctx.Catalog = cat

	err := structurePages(ctx, "Sales")
	if err == nil {
		t.Fatal("structurePages swallowed a failing catalog query")
	}
	if !strings.Contains(err.Error(), "widgets") {
		t.Errorf("error should name the failing query; got: %v", err)
	}
}
