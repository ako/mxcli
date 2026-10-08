// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1338: a DataGrid 2 column bound to `changedBy/Name` on an
// entity declaring `changedBy: AutoChangedBy` passed check and exec, and mxbuild
// then failed
//
//	[CE1613] "The selected attribute 'MyFirstModule.Item.changedBy/Name' no longer exists."
//
// changedBy and owner are not attributes: they are the ASSOCIATIONS
// System.changedBy / System.owner to System.User, which no domain model lists —
// an entity carries them as the HasChangedBy / HasOwner flags. The path resolver
// looked the hop up as `MyFirstModule.changedBy`, found nothing, and the caller
// fell back to a flat attribute path.
func TestResolveAssociationAttributePath_SystemAssociations(t *testing.T) {
	const (
		modID   = model.ID("mod-my")
		itemID  = model.ID("e-item")
		subID   = model.ID("e-sub")
		plainID = model.ID("e-plain")
		userID  = model.ID("e-person")
	)

	newPB := func(ctx string) *pageBuilder {
		return &pageBuilder{
			entityContext: ctx,
			execCache: &executorCache{
				hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "MyFirstModule"}},
				domainModels: []*domainmodel.DomainModel{{
					ContainerID: modID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: itemID}, Name: "Item", HasChangedBy: true, HasOwner: true},
						// Inherits Item's flags: Mendix keeps them on the root only.
						{BaseElement: model.BaseElement{ID: subID}, Name: "SubItem", GeneralizationRef: "MyFirstModule.Item"},
						{BaseElement: model.BaseElement{ID: plainID}, Name: "Plain"},
						{BaseElement: model.BaseElement{ID: userID}, Name: "Person"},
					},
					Associations: []*domainmodel.Association{
						// A user association that happens to be called `owner`
						// on an entity that does not store System.owner.
						{Name: "owner", ParentID: plainID, ChildID: userID, Type: domainmodel.AssociationTypeReference},
					},
				}},
			},
		}
	}

	tests := []struct {
		name      string
		ctx       string
		path      string
		wantFinal string
		wantAssoc string
		wantDest  string
	}{
		{"changedBy (the report)", "MyFirstModule.Item", "changedBy/Name", "System.User.Name", "System.changedBy", "System.User"},
		{"owner", "MyFirstModule.Item", "owner/Name", "System.User.Name", "System.owner", "System.User"},
		{"qualified spelling", "MyFirstModule.Item", "System.changedBy/Name", "System.User.Name", "System.changedBy", "System.User"},
		{"stored spelling is canonical", "MyFirstModule.Item", "ChangedBy/Name", "System.User.Name", "System.changedBy", "System.User"},
		{"inherited from the root", "MyFirstModule.SubItem", "changedBy/Name", "System.User.Name", "System.changedBy", "System.User"},
		{"user association wins", "MyFirstModule.Plain", "owner/FullName", "MyFirstModule.Person.FullName", "MyFirstModule.owner", "MyFirstModule.Person"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			finalQN, steps, ok := newPB(tc.ctx).resolveAssociationAttributePath(tc.path)
			if !ok {
				t.Fatalf("path %q was dropped (ok=false) — the binding falls back to a flat path and mxbuild fails CE1613", tc.path)
			}
			if finalQN != tc.wantFinal {
				t.Errorf("finalQN = %q, want %q", finalQN, tc.wantFinal)
			}
			if len(steps) != 1 || steps[0].Association != tc.wantAssoc || steps[0].DestinationEntity != tc.wantDest {
				t.Errorf("steps = %+v, want one {Association: %s, DestinationEntity: %s}", steps, tc.wantAssoc, tc.wantDest)
			}
		})
	}

	// An entity that does not store the member has no such association; inventing
	// the hop would write a reference mxbuild rejects just the same.
	t.Run("entity without changedBy", func(t *testing.T) {
		if _, steps, ok := newPB("MyFirstModule.Plain").resolveAssociationAttributePath("changedBy/Name"); ok {
			t.Errorf("resolved %+v for an entity that does not store changedBy", steps)
		}
	})
}
