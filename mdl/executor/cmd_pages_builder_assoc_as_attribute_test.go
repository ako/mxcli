// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// upstream #830: `column colCustomer (attribute: Order_Customer)` — an
// ASSOCIATION given to an attribute-typed widget property — was qualified like
// an attribute and written as
//
//	DomainModels$AttributeRef{Attribute: "ZKT38.Order.Order_Customer"}
//
// so mxbuild failed CE1613 "The selected attribute … no longer exists".
//
// This is a refusal rather than a fix because the reference is NOT
// representable, established against mxbuild 11.13.0:
//   - CustomWidgets$WidgetValue.AttributeRef is typed `AttributeRef`, not the
//     polymorphic `MemberRef`. Hand-patching a DomainModels$AssociationRef into
//     it makes the project UNLOADABLE — `mx check` dies before validation with
//     "Object of type 'AssociationRef' cannot be converted to type
//     'AttributeRef'".
//   - The WidgetValue carries no association-valued property at all:
//     Mendix.Modeler.WebUI.dll, which defines the type, has no `AssociationRef`
//     member.
//
// The DataGrid column property's <associationTypes>Reference/ReferenceSet is
// what makes this look supported. It permits the attribute PATH to TRAVERSE a
// reference (`Order_Customer/Name`), which mxcli already writes correctly — so
// the accepted cases below matter as much as the refused one.
func TestRejectAssociationAsAttribute(t *testing.T) {
	const (
		modID      = model.ID("mod-zkt")
		orderID    = model.ID("e-order")
		customerID = model.ID("e-customer")
		baseID     = model.ID("e-base")
	)

	newPB := func() *pageBuilder {
		return &pageBuilder{
			entityContext: "ZKT38.Order",
			execCache: &executorCache{
				hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "ZKT38"}},
				domainModels: []*domainmodel.DomainModel{
					{
						ContainerID: modID,
						Entities: []*domainmodel.Entity{
							{
								BaseElement:       model.BaseElement{ID: orderID},
								Name:              "Order",
								GeneralizationRef: "ZKT38.Base",
								Attributes: []*domainmodel.Attribute{
									{Name: "Number"},
								},
							},
							{
								BaseElement: model.BaseElement{ID: customerID},
								Name:        "Customer",
								Attributes:  []*domainmodel.Attribute{{Name: "Name"}},
							},
							{
								BaseElement: model.BaseElement{ID: baseID},
								Name:        "Base",
								Attributes:  []*domainmodel.Attribute{{Name: "Code"}},
							},
						},
						Associations: []*domainmodel.Association{
							{Name: "Order_Customer", ParentID: orderID, ChildID: customerID, Type: domainmodel.AssociationTypeReference},
							// Declared on the GENERALIZATION: reachable from Order,
							// and just as unrepresentable there.
							{Name: "Base_Customer", ParentID: baseID, ChildID: customerID, Type: domainmodel.AssociationTypeReference},
						},
					},
				},
			},
		}
	}

	cases := []struct {
		name     string
		binding  string
		wantRefs bool
	}{
		{"the reported form: a reference bound as an attribute", "Order_Customer", true},
		{"an association declared on the generalization", "Base_Customer", true},

		// Everything below must still be accepted — a refusal here would break
		// working pages.
		{"a plain attribute", "Number", false},
		{"an inherited attribute", "Code", false},
		{"the SUPPORTED traversal form", "Order_Customer/Name", false},
		{"an explicit three-part attribute", "ZKT38.Order.Number", false},
		{"a variable binding", "$Order", false},
		{"an empty binding", "", false},
		{"a name that is neither", "NoSuchMember", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pb := newPB()
			err := pb.rejectAssociationAsAttribute(tc.binding, pb.entityContext, "column `c` property `attribute`")
			if !tc.wantRefs {
				if err != nil {
					t.Fatalf("binding %q must be accepted, got: %v", tc.binding, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("binding %q is an association and must be refused — "+
					"writing it produces CE1613 and there is no correct BSON to write instead", tc.binding)
			}
			// The message has to carry both escape routes: the traversal form for
			// showing a value, and the filter's association mode for filtering.
			for _, want := range []string{"is an association", "CE1613", tc.binding + "/<Attr>", "dropdownfilter"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q, got: %v", want, err)
				}
			}
		})
	}
}

// Without a model there is no way to tell an association from an attribute, and
// guessing would reject valid MDL. Widget builders that run in isolation (unit
// tests, and any caller with no backend and nothing cached) must pass through.
func TestRejectAssociationAsAttribute_NoModelPassesThrough(t *testing.T) {
	pb := &pageBuilder{entityContext: "ZKT38.Order"}
	if err := pb.rejectAssociationAsAttribute("Order_Customer", "ZKT38.Order", "column `c`"); err != nil {
		t.Fatalf("with no model available the binding must pass through, got: %v", err)
	}
}
