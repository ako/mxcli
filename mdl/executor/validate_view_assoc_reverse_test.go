// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// viewAssocCtx is a project with a persistent C1611.Customer, a C1611.Item
// pointing at it, and a view entity C1611.Stats whose OQL selects the
// customer's id as Stats_Customer — the association Mendix derives from it.
func viewAssocCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-1611"}, Name: "C1611"}
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: "dm-1611"}, ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: "e-cust"}, Name: "Customer", Persistable: true},
					{BaseElement: model.BaseElement{ID: "e-item"}, Name: "Item", Persistable: true},
					{BaseElement: model.BaseElement{ID: "e-stats"}, Name: "Stats", Persistable: true,
						Source: "DomainModels$OqlViewEntitySource"},
				},
				Associations: []*domainmodel.Association{
					{BaseElement: model.BaseElement{ID: "a-item"}, Name: "Item_Customer", ParentID: "e-item", ChildID: "e-cust",
						Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault},
					{BaseElement: model.BaseElement{ID: "a-stats"}, Name: "Stats_Customer", ParentID: "e-stats", ChildID: "e-cust",
						Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault,
						Source: domainmodel.OqlViewAssociationSource, ViewSourceReference: "Stats_Customer"},
				},
			}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(b))
	ctx.widgetRegistry = &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{}, byWidgetID: map[string]*WidgetDefinition{}}
	ctx.widgetRegistryLoaded = true
	return ctx
}

func viewAssocPage(param, widget, ds string) string {
	return `create page C1611.P (title: 'P', layout: Atlas_Core.Atlas_Default, params: ($P: C1611.` + param + `)) {
  dataview dv (datasource: $P) {
    ` + widget + ` w (datasource: ` + ds + `) { }
  }
}`
}

// mendixlabs/mxcli#1611 A: a page data source that follows a view entity's
// association from the entity it points at passed check and exec, then mxbuild
// failed CE8812 "Entity 'M.Owner' has no association 'M.Stats_Owner'". Each
// case is one measured shape (11.14.0, one page per shape); the controls built
// at 0 errors.
func TestViewAssociationReverse_MeasuredShapes(t *testing.T) {
	ctx := viewAssocCtx(t)
	for _, tc := range []struct {
		name, param, widget, ds string
		ce8812                  bool
	}{
		{"database from, the report", "Customer", "listview", "DATABASE FROM $currentObject/C1611.Stats_Customer/C1611.Stats", true},
		{"in-memory association source", "Customer", "listview", "$currentObject/C1611.Stats_Customer", true},
		{"data grid", "Customer", "datagrid", "DATABASE FROM $currentObject/C1611.Stats_Customer/C1611.Stats", true},
		{"gallery", "Customer", "gallery", "DATABASE FROM $currentObject/C1611.Stats_Customer/C1611.Stats", true},
		{"data view", "Customer", "dataview", "$currentObject/C1611.Stats_Customer", true},
		{"second hop", "Item", "listview", "DATABASE FROM $currentObject/C1611.Item_Customer/C1611.Customer/C1611.Stats_Customer/C1611.Stats", true},
		{"unqualified association", "Customer", "listview", "$currentObject/Stats_Customer", true},
		// controls
		{"persistent association reversed", "Customer", "listview", "DATABASE FROM $currentObject/C1611.Item_Customer/C1611.Item", false},
		{"view association forward", "Stats", "dataview", "$currentObject/C1611.Stats_Customer", false},
		{"forward then a persistent reverse", "Stats", "listview", "DATABASE FROM $currentObject/C1611.Stats_Customer/C1611.Customer/C1611.Item_Customer/C1611.Item", false},
		{"the reporter's workaround", "Customer", "listview", "DATABASE C1611.Stats WHERE [C1611.Stats_Customer = $currentObject]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := assocListErrs(t, ctx, viewAssocPage(tc.param, tc.widget, tc.ds))
			var hits []string
			for _, e := range errs {
				if strings.Contains(e, "MDL-ASSOCDS02") {
					hits = append(hits, e)
				}
			}
			if !tc.ce8812 {
				if len(errs) != 0 {
					t.Errorf("want no error, got %q", errs)
				}
				return
			}
			if len(hits) != 1 || len(errs) != 1 {
				t.Fatalf("want one MDL-ASSOCDS02, got %q", errs)
			}
			for _, want := range []string{"CE8812", "C1611.Stats_Customer", "view entity C1611.Stats",
				"DATABASE C1611.Stats WHERE [C1611.Stats_Customer = "} {
				if !strings.Contains(hits[0], want) {
					t.Errorf("want %q in %q", want, hits[0])
				}
			}
		})
	}
}

// The reporter's flow creates the view entity and the page in one script: the
// association comes from the view's OQL, not from a CREATE ASSOCIATION.
func TestViewAssociationReverse_ScriptDeclaredView(t *testing.T) {
	src := `create view entity C1611.Totals (Total: Integer) as (
  from C1611.Item as i
  join i/C1611.Item_Customer/C1611.Customer as c
  group by c.ID
  select c.ID as Totals_Customer, count(i.ID) as Total
);
` + viewAssocPage("Customer", "listview", "DATABASE FROM $currentObject/C1611.Totals_Customer/C1611.Totals")
	errs := assocListErrs(t, viewAssocCtx(t), src)
	if len(errs) != 1 || !strings.Contains(errs[0], "MDL-ASSOCDS02") || !strings.Contains(errs[0], "C1611.Totals_Customer") {
		t.Errorf("want one MDL-ASSOCDS02 for the script's view association, got %q", errs)
	}
}

// The remedy MDL-ASSOCDS02 names — query the view entity, constrained on its
// association — must itself pass when the view entity is created by the same
// script. It did not: the script's view associations were known to no checker,
// so `[C1611.Totals_Customer = $currentObject]` was refused as naming "neither
// an attribute nor an association", on a page and in a microflow retrieve,
// while mxbuild builds both at 0 errors (measured, 11.14.0).
func TestViewAssociationReverse_RemedyPassesForScriptDeclaredView(t *testing.T) {
	src := `create view entity C1611.Totals (Total: Integer) as (
  from C1611.Item as i
  join i/C1611.Item_Customer/C1611.Customer as c
  group by c.ID
  select c.ID as Totals_Customer, count(i.ID) as Total
);
` + viewAssocPage("Customer", "listview", "DATABASE C1611.Totals WHERE [C1611.Totals_Customer = $currentObject]") + `;
create microflow C1611.MF ($C: C1611.Customer) begin retrieve $L from C1611.Totals where C1611.Totals_Customer = $C; end;`
	prog := parseMDL(t, src)
	for _, err := range validateProgram(viewAssocCtx(t), prog) {
		if strings.Contains(err.Error(), "Totals_Customer") {
			t.Errorf("the remedy was refused: %v", err)
		}
	}
}
