// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#553 — DROP ENTITY left every CROSS-MODULE association pointing at the
// deleted entity in place.
//
// The cascade in DeleteEntity swept `dm.AssociationsItems()` and asserted
// `*genDm.Association` on each item, so the separate CrossAssociations collection
// was never looked at. Dropping the local (by-id) end left a 16-byte pointer to an
// element that no longer exists, and mxbuild 11.14.0 then could not LOAD the
// project at all:
//
//	ERROR: System.AggregateException … (The given key
//	'49751a65-d5f9-456c-887e-3f14bacb8822' was not present in the dictionary.)
//	  at StreamingBsonUnitReader.ResolvePostponedProperties()
//
// No CE code, no document named — the failure is in the storage layer, upstream of
// the consistency checker, so the obvious reading is "the project is corrupt".
// Dropping the by-name end is milder and still wrong: CE1613 at the cross-module
// association.
//
// It was reported against a view entity (whose associations are DERIVED from its
// OQL, so there is no CREATE ASSOCIATION to undo), but nothing here is specific to
// view entities: a plain cross-module association orphans identically. The
// single-module case was always handled, which is why this went unnoticed.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func TestDeleteEntity_RemovesCrossModuleAssociations(t *testing.T) {
	// Both ends of a cross-module association, each deleted in its own project so
	// the two directions cannot mask each other.
	for _, tc := range []struct {
		name string
		// drop reports which entity to delete: the local by-id FROM end, or the
		// by-name TO end in the other module.
		dropChild bool
	}{
		{"the by-id FROM end (mxbuild cannot load the project)", false},
		{"the by-name TO end (CE1613 at the cross-module association)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = b.Disconnect() })

			fromMod, err := b.GetModuleByName("MyFirstModule")
			if err != nil || fromMod == nil {
				t.Fatalf("GetModuleByName(MyFirstModule): %v", err)
			}
			toMod, err := b.GetModuleByName("Administration")
			if err != nil || toMod == nil {
				t.Fatalf("GetModuleByName(Administration): %v", err)
			}
			fromDM, err := b.GetDomainModel(fromMod.ID)
			if err != nil {
				t.Fatalf("GetDomainModel(from): %v", err)
			}
			toDM, err := b.GetDomainModel(toMod.ID)
			if err != nil {
				t.Fatalf("GetDomainModel(to): %v", err)
			}

			from := &domainmodel.Entity{Name: "ZzOrder", Persistable: true}
			if err := b.CreateEntity(fromDM.ID, from); err != nil {
				t.Fatalf("CreateEntity(from): %v", err)
			}
			to := &domainmodel.Entity{Name: "ZzCustomer", Persistable: true}
			if err := b.CreateEntity(toDM.ID, to); err != nil {
				t.Fatalf("CreateEntity(to): %v", err)
			}
			// A second, untouched cross association is the control: the sweep must
			// remove what the deleted entity holds up and nothing else.
			keep := &domainmodel.Entity{Name: "ZzKeeper", Persistable: true}
			if err := b.CreateEntity(fromDM.ID, keep); err != nil {
				t.Fatalf("CreateEntity(keep): %v", err)
			}

			for _, ca := range []*domainmodel.CrossModuleAssociation{
				{Name: "ZzOrder_Customer", ParentID: from.ID, ChildRef: "Administration.ZzCustomer",
					Type: "Reference", Owner: "Default", StorageFormat: "Column"},
				{Name: "ZzKeeper_Account", ParentID: keep.ID, ChildRef: "Administration.Account",
					Type: "Reference", Owner: "Default", StorageFormat: "Column"},
			} {
				if err := b.CreateCrossAssociation(fromDM.ID, ca); err != nil {
					t.Fatalf("CreateCrossAssociation(%s): %v", ca.Name, err)
				}
			}

			if tc.dropChild {
				if err := b.DeleteEntity(toDM.ID, to.ID); err != nil {
					t.Fatalf("DeleteEntity(to): %v", err)
				}
			} else {
				if err := b.DeleteEntity(fromDM.ID, from.ID); err != nil {
					t.Fatalf("DeleteEntity(from): %v", err)
				}
			}

			after, err := b.GetDomainModel(fromMod.ID)
			if err != nil {
				t.Fatalf("GetDomainModel after delete: %v", err)
			}
			var names []string
			for _, ca := range after.CrossAssociations {
				names = append(names, ca.Name)
				if ca.Name == "ZzOrder_Customer" {
					t.Errorf("cross-module association %q survived the delete — it now points at an entity "+
						"that does not exist, which mxbuild reports as a KeyNotFoundException at "+
						"ResolvePostponedProperties (by id) or CE1613 (by name)", ca.Name)
				}
			}
			var keptFound bool
			for _, n := range names {
				if n == "ZzKeeper_Account" {
					keptFound = true
				}
			}
			if !keptFound {
				t.Errorf("the unrelated cross-module association was removed too (remaining: %v) — "+
					"the sweep must match the deleted entity, not clear the collection", names)
			}
		})
	}
}
