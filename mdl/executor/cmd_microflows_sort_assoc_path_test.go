// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1152, second half — the residue inference leaves behind.
//
// Two associations reach the same entity (Order_ShipTo and Order_BillTo, both
// Order -> Address), which is an ordinary shape, not a corner case. The stored
// hop is then not recoverable from the sort attribute's name, so DESCRIBE
// dropping it means the replay has to guess. Measured on 11.12.3: a microflow
// sorting by the BILLING address came back from `describe -> exec` sorting by
// the SHIPPING one, at 0 errors on both sides.
//
// `sort by Mod.Order_BillTo/Mod.Address.City` is the spelling that removes the
// guess. These tests pin the resolution; the describe half is pinned in
// mdl/backend/modelsdk (the read) and by the formatter test below.
func twoHopBackend() *mock.MockBackend {
	moduleID := model.ID("synthetic-sales-module")
	order := &domainmodel.Entity{Name: "Order"}
	order.ID = model.ID("Sales.Order")
	address := &domainmodel.Entity{
		Name: "Address",
		Attributes: []*domainmodel.Attribute{
			{Name: "City", Type: &domainmodel.StringAttributeType{}},
		},
	}
	address.ID = model.ID("Sales.Address")
	shipTo := &domainmodel.Association{Name: "Order_ShipTo", ParentID: order.ID, ChildID: address.ID, Type: domainmodel.AssociationTypeReference}
	shipTo.ID = model.ID("Sales.Order_ShipTo")
	billTo := &domainmodel.Association{Name: "Order_BillTo", ParentID: order.ID, ChildID: address.ID, Type: domainmodel.AssociationTypeReference}
	billTo.ID = model.ID("Sales.Order_BillTo")

	return &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Sales" {
				return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id != moduleID {
				return nil, nil
			}
			return &domainmodel.DomainModel{
				ContainerID:  moduleID,
				Entities:     []*domainmodel.Entity{order, address},
				Associations: []*domainmodel.Association{shipTo, billTo},
			}, nil
		},
	}
}

// pathSortOf builds the retrieve for an authored sort column and returns the
// stored attribute path, its hops, and any build errors.
func pathSortOf(t *testing.T, col ast.SortColumnDef) (path string, steps []microflows.EntityRefStep, errs []string) {
	t.Helper()
	fb := &flowBuilder{backend: twoHopBackend(), spacing: 100}
	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:    "Orders",
		Source:      ast.QualifiedName{Module: "Sales", Name: "Order"},
		SortColumns: []ast.SortColumnDef{col},
	})
	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		ra, ok := act.Action.(*microflows.RetrieveAction)
		if !ok {
			continue
		}
		ds, ok := ra.Source.(*microflows.DatabaseRetrieveSource)
		if !ok {
			continue
		}
		for _, s := range ds.Sorting {
			path, steps = s.AttributeQualifiedName, s.EntityRefSteps
		}
	}
	return path, steps, fb.errors
}

// The named association is the one stored — not the first one that happens to
// reach Address. This is the whole point of the spelling.
func TestSortPath_NamedAssociationIsTheOneStored(t *testing.T) {
	path, steps, errs := pathSortOf(t, ast.SortColumnDef{
		Associations: []string{"Sales.Order_BillTo"},
		Attribute:    "Sales.Address.City",
		Order:        "ASC",
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if path != "Sales.Address.City" {
		t.Errorf("stored attribute %q, want Sales.Address.City", path)
	}
	if len(steps) != 1 || steps[0].Association != "Sales.Order_BillTo" || steps[0].DestinationEntity != "Sales.Address" {
		t.Fatalf("stored hops %+v, want one Sales.Order_BillTo -> Sales.Address", steps)
	}
}

// CONTROL: the sibling association is equally reachable, so a resolver that
// ignored the name and searched the domain model would pass the test above by
// luck. Asking for the other one must store the other one.
func TestSortPath_TheOtherAssociationStoresTheOther(t *testing.T) {
	_, steps, errs := pathSortOf(t, ast.SortColumnDef{
		Associations: []string{"Sales.Order_ShipTo"},
		Attribute:    "Sales.Address.City",
		Order:        "ASC",
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(steps) != 1 || steps[0].Association != "Sales.Order_ShipTo" {
		t.Fatalf("stored hops %+v, want one Sales.Order_ShipTo", steps)
	}
}

// An unqualified hop is looked up in the entity's own module.
func TestSortPath_UnqualifiedHopAndBareAttribute(t *testing.T) {
	path, steps, errs := pathSortOf(t, ast.SortColumnDef{
		Associations: []string{"Order_BillTo"},
		Attribute:    "City",
		Order:        "ASC",
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if path != "Sales.Address.City" {
		t.Errorf("stored attribute %q, want Sales.Address.City — a bare attribute after a hop "+
			"is qualified with the entity the hop reaches", path)
	}
	if len(steps) != 1 || steps[0].Association != "Sales.Order_BillTo" {
		t.Fatalf("stored hops %+v, want one Sales.Order_BillTo", steps)
	}
}

// A hop that does not exist is refused, not written with an empty destination —
// a step with no DestinationEntity makes the project unopenable rather than
// merely wrong.
func TestSortPath_UnknownHopIsRefused(t *testing.T) {
	_, steps, errs := pathSortOf(t, ast.SortColumnDef{
		Associations: []string{"Sales.Order_Nowhere"},
		Attribute:    "Sales.Address.City",
		Order:        "ASC",
	})
	if len(errs) == 0 {
		t.Fatal("an unknown association must be refused")
	}
	if len(steps) != 0 {
		t.Errorf("a refused hop must store nothing, got %+v", steps)
	}
	if !strings.Contains(strings.Join(errs, " "), "was not found") {
		t.Errorf("unexpected message: %v", errs)
	}
}

// An attribute that is not on the entity the path ends at is refused.
func TestSortPath_AttributeOffThePathIsRefused(t *testing.T) {
	_, _, errs := pathSortOf(t, ast.SortColumnDef{
		Associations: []string{"Sales.Order_BillTo"},
		Attribute:    "Sales.Order.OrderNo",
		Order:        "ASC",
	})
	if len(errs) == 0 {
		t.Fatal("an attribute off the end of the path must be refused")
	}
	if !strings.Contains(strings.Join(errs, " "), "does not belong to") {
		t.Errorf("unexpected message: %v", errs)
	}
}
