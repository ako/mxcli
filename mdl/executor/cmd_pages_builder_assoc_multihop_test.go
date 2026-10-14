// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1467: a data view over two associations from a page
// parameter. The builder split the path at the first `/`, so
//
//   - the short form `$Order/Mod.Order_Account/Mod.Account_Flag` (what describe
//     printed) stored the second ASSOCIATION as the destination entity —
//     CE1613 "The selected entity 'Mod.Account_Flag' no longer exists";
//   - the full form `$Order/Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag`
//     stored "Mod.Account/Mod.Account_Flag/Mod.Flag" as ONE destination, and
//     the project would not load ("error … set the 'DestinationEntity' property
//     of a Entity ref step").
//
// Both must resolve to one association/destination pair per hop.
func assocHopsPB() *pageBuilder {
	const (
		modID     = model.ID("mod")
		orderID   = model.ID("e-order")
		accountID = model.ID("e-account")
		flagID    = model.ID("e-flag")
	)
	return &pageBuilder{
		paramEntityNames: map[string]string{"Order": "Mod.Order"},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Mod"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: orderID}, Name: "Order"},
					{BaseElement: model.BaseElement{ID: accountID}, Name: "Account"},
					{BaseElement: model.BaseElement{ID: flagID}, Name: "Flag"},
				},
				Associations: []*domainmodel.Association{
					{Name: "Order_Account", ParentID: orderID, ChildID: accountID, Type: domainmodel.AssociationTypeReference},
					{Name: "Account_Flag", ParentID: accountID, ChildID: flagID, Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerBoth},
				},
			}},
		},
	}
}

func TestAssociationDataSource_MultiHop(t *testing.T) {
	const want = "Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag"
	for _, ref := range []string{
		"Mod.Order_Account/Mod.Account_Flag",                      // short form (describe's old output)
		"Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag", // full form
		"Mod.Order_Account/Mod.Account/Mod.Account_Flag",          // mixed: trailing destination derived
		"Mod.Order_Account/Mod.Account_Flag/Mod.Flag",             // mixed: first destination derived
		"Order_Account/Account_Flag",                              // bare names qualified per hop
	} {
		t.Run(ref, func(t *testing.T) {
			ds, childCtx, err := assocHopsPB().buildDataSourceV3(&ast.DataSourceV3{
				Type: "association", ContextVariable: "Order", Reference: ref,
			})
			if err != nil {
				t.Fatalf("buildDataSourceV3(%q) = error %v", ref, err)
			}
			src, ok := ds.(*pages.AssociationSource)
			if !ok {
				t.Fatalf("got %T, want *pages.AssociationSource", ds)
			}
			if src.EntityPath != want {
				t.Errorf("EntityPath = %q, want %q", src.EntityPath, want)
			}
			if childCtx != "Mod.Flag" {
				t.Errorf("child entity context = %q, want Mod.Flag", childCtx)
			}
		})
	}
}

// A destination that is not an end of its association is neither the next hop
// nor a valid destination — refuse rather than write a by-name entity
// reference Mendix resolves to null (or to the wrong entity).
func TestAssociationDataSource_MultiHop_RejectsWrongDestination(t *testing.T) {
	for _, ref := range []string{
		"Mod.Order_Account/Mod.Flag/Mod.Account_Flag",   // Flag is not an end of Order_Account
		"Mod.Order_Account/Mod.Account/Mod.Acount_Flag", // misspelled second association
	} {
		t.Run(ref, func(t *testing.T) {
			_, _, err := assocHopsPB().buildDataSourceV3(&ast.DataSourceV3{
				Type: "association", ContextVariable: "Order", Reference: ref,
			})
			if err == nil {
				t.Fatalf("buildDataSourceV3(%q) succeeded; want a refusal", ref)
			}
			if !strings.Contains(err.Error(), ref) {
				t.Errorf("error %q does not quote the datasource path", err)
			}
		})
	}
}

// Describe must print a multi-hop path that executes back to the same steps:
// every hop's stored destination, not the associations alone.
func TestAssociationSourcePath_MultiHopPrintsDestinations(t *testing.T) {
	ds := map[string]any{
		"$Type": "Forms$DataViewSource",
		"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "Mod.Order_Account", "DestinationEntity": "Mod.Account"},
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "Mod.Account_Flag", "DestinationEntity": "Mod.Flag"},
		}},
		"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Order"},
	}
	path, ctx := associationSourcePath(ds)
	if want := "Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag"; path != want || ctx != "Order" {
		t.Errorf("associationSourcePath = (%q, %q), want (%q, Order)", path, ctx, want)
	}
	// And the printed path builds back to the same steps.
	built, _, err := assocHopsPB().buildDataSourceV3(&ast.DataSourceV3{Type: "association", ContextVariable: ctx, Reference: path})
	if err != nil {
		t.Fatalf("re-executing the described path: %v", err)
	}
	if got := built.(*pages.AssociationSource).EntityPath; got != path {
		t.Errorf("round trip EntityPath = %q, want %q", got, path)
	}
}
