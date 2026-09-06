// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// An association datasource at PAGE level typed its rows as the entity it was
// navigating AWAY from (mendixlabs/mxcli#1045):
//
//	datagrid gA (datasource: $Customer/Bench.Order_Customer) { … OrderNo … }
//	mx check -> [CE1613] "The selected attribute 'Bench.Customer.OrderNo'
//	            no longer exists." at Columns (1/1) of data grid 2 'gA'
//
// The destination is resolved as "the end opposite the context", and the context
// used was pb.entityContext — the ENCLOSING data container's entity. That is
// right inside a data view and empty at page level, where nothing encloses the
// widget, so neither end matched and the last-resort fallback returned the TO
// side. A named context variable answers the question directly.

// assocPageBuilder is Order --Order_Customer--> Customer, with $Customer a page
// parameter. Order is the FROM (parent) end, Customer the TO (child) end, so
// traversing FROM Customer must reach Order.
func assocPageBuilder(entityContext string) *pageBuilder {
	const (
		modID   = model.ID("mod-bench")
		orderID = model.ID("e-order")
		custID  = model.ID("e-cust")
	)
	return &pageBuilder{
		entityContext: entityContext,
		paramEntityNames: map[string]string{
			"Customer": "Bench.Customer",
		},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Bench"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: orderID}, Name: "Order"},
					{BaseElement: model.BaseElement{ID: custID}, Name: "Customer"},
				},
				Associations: []*domainmodel.Association{
					{Name: "Order_Customer", ParentID: orderID, ChildID: custID,
						Type: domainmodel.AssociationTypeReference},
				},
			}},
		},
	}
}

// THE REGRESSION. At page level there is no enclosing container, so the only
// thing that can say what the association is traversed from is the variable the
// author named.
func TestAssociationDataSource_PageLevelResolvesFromTheNamedVariable(t *testing.T) {
	// entityContext is empty: nothing encloses a page-level widget.
	ds, childCtx, err := assocPageBuilder("").buildDataSourceV3(&ast.DataSourceV3{
		Type:            "association",
		Reference:       "Bench.Order_Customer",
		ContextVariable: "Customer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if childCtx != "Bench.Order" {
		t.Errorf("rows typed as %q, want Bench.Order — the grid navigates FROM "+
			"Customer, so its rows are the other end", childCtx)
	}
	assertEntityPath(t, ds, "Bench.Order_Customer/Bench.Order")
}

// CONTROL: the data-view case must be unchanged. `$currentObject/Assoc` names
// no variable, so the enclosing entity is still what answers — and it did
// answer correctly before this fix, which is why the report singled out page
// level.
func TestAssociationDataSource_DataViewStillUsesTheEnclosingEntity(t *testing.T) {
	ds, childCtx, err := assocPageBuilder("Bench.Customer").buildDataSourceV3(&ast.DataSourceV3{
		Type:            "association",
		Reference:       "Bench.Order_Customer",
		ContextVariable: "currentObject",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if childCtx != "Bench.Order" {
		t.Errorf("rows typed as %q, want Bench.Order", childCtx)
	}
	assertEntityPath(t, ds, "Bench.Order_Customer/Bench.Order")
}

// CONTROL: a variable the builder does not know falls back to the enclosing
// entity, exactly as before. Guessing from an unknown name would be worse than
// the behaviour this replaces.
func TestAssociationDataSource_UnknownVariableFallsBackToTheEnclosingEntity(t *testing.T) {
	_, childCtx, err := assocPageBuilder("Bench.Customer").buildDataSourceV3(&ast.DataSourceV3{
		Type:            "association",
		Reference:       "Bench.Order_Customer",
		ContextVariable: "SomethingElse",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if childCtx != "Bench.Order" {
		t.Errorf("rows typed as %q, want Bench.Order via the enclosing entity", childCtx)
	}
}

// CONTROL: traversing the OTHER way still resolves the other way. Without this,
// a fix that always returned the FROM end would pass the tests above.
func TestAssociationDataSource_ReverseDirectionStillResolves(t *testing.T) {
	pb := assocPageBuilder("")
	pb.paramEntityNames = map[string]string{"Order": "Bench.Order"}

	_, childCtx, err := pb.buildDataSourceV3(&ast.DataSourceV3{
		Type:            "association",
		Reference:       "Bench.Order_Customer",
		ContextVariable: "Order",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if childCtx != "Bench.Customer" {
		t.Errorf("rows typed as %q, want Bench.Customer — traversed from Order, "+
			"the destination is the other end", childCtx)
	}
}

// assertEntityPath checks the EntityPath the AssociationSource carries, which is
// what Mendix stores and therefore what mxbuild reads.
func assertEntityPath(t *testing.T, ds any, want string) {
	t.Helper()
	src, ok := ds.(*pages.AssociationSource)
	if !ok {
		t.Fatalf("datasource is %T, want *pages.AssociationSource", ds)
	}
	if src.EntityPath != want {
		t.Errorf("EntityPath = %q, want %q", src.EntityPath, want)
	}
}
