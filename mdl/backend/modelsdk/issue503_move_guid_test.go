// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue503_MovePreservesStorageGUIDs guards ako/mxcli#503: MOVE ENTITY must
// not re-mint a storage GUID — not the moved entity's, not its attributes', and
// not that of an association the move converts to a cross-association.
//
// #657 fixed the entity GUID for UpdateEntity and #1119 its children; MoveEntity
// got neither, and additionally dropped the association's GUID when converting
// it. That last one is why the #1119 write guard began refusing MOVE ENTITY for
// any entity in an association: the conversion happens in place in the source
// unit, keeping the element's $ID, so the guard could see the GUID move. The
// other losses are invisible to it — the entity lands in a DIFFERENT unit, where
// its $ID matches nothing stored, and cross-unit moves are outside the guard's
// reach by construction.
//
// Both endpoints are covered because the conversion is asymmetric: moving the
// CHILD leaves the cross-association in the source unit, moving the PARENT sends
// it to the target.
func TestIssue503_MovePreservesStorageGUIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		// endpoint picks which side of the association to move: the child (TO)
		// entity or the parent (FROM) entity.
		moveChild bool
		// assocLandsIn says which domain model should hold the cross-association
		// afterwards, so the assertion looks in the right unit.
		assocInSource bool
	}{
		{name: "MoveChild_CrossAssocStaysInSource", moveChild: true, assocInSource: true},
		{name: "MoveParent_CrossAssocGoesToTarget", moveChild: false, assocInSource: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}

			srcID, srcMod, assocName, assocGUIDBefore, childID, parentID := associationSubject(t, b)
			dstID, dstMod := otherDomainModel(t, b, srcID)
			if dstID == "" {
				t.Skip("fixture has no second loadable domain model to move into")
			}

			entID := childID
			if !tc.moveChild {
				entID = parentID
			}
			ent := entityByID(t, b, srcID, entID)
			if len(ent.Attributes) == 0 {
				t.Skipf("subject entity %s has no attributes; nothing to detect the conflation with", ent.Name)
			}

			// Preconditions: without GUID != $ID on both the association and the
			// attributes, this test cannot fail against the broken code.
			attrsBefore := attributeGUIDs(t, b, srcID, entID)
			attrIDs := attributeIDs(t, b, srcID, entID)
			for name, guid := range attrsBefore {
				if guid == "" || guid == attrIDs[name] {
					t.Fatalf("fixture attribute %s has GUID %q and $ID %q; need them to differ", name, guid, attrIDs[name])
				}
			}
			entGUIDBefore := entityGUID(t, b, srcID, entID)
			entIDHex := entityRawKey(t, b, srcID, entID, "$ID")
			if entGUIDBefore == "" || entGUIDBefore == entIDHex {
				t.Fatalf("fixture entity %s has GUID %q and $ID %q; need them to differ", ent.Name, entGUIDBefore, entIDHex)
			}
			if assocGUIDBefore == "" {
				t.Fatalf("association %s has no GUID; cannot verify preservation", assocName)
			}

			converted, err := b.MoveEntity(ent, srcID, dstID, srcMod, dstMod)
			if err != nil {
				t.Fatalf("MoveEntity: %v", err)
			}
			if len(converted) == 0 {
				t.Fatalf("expected %s to be converted to a cross-association, got %v", assocName, converted)
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			// Reopen: preservation only counts if it reached disk.
			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })

			// The move must actually have happened — preserving GUIDs on an entity
			// that never moved proves nothing.
			if e := findEntityByName(t, b2, dstID, ent.Name); e == nil {
				t.Fatalf("entity %s is not in the target domain model after the move", ent.Name)
			}
			if e := findEntityByName(t, b2, srcID, ent.Name); e != nil {
				t.Errorf("entity %s is still in the source domain model after the move", ent.Name)
			}

			if got := entityGUID(t, b2, dstID, entID); got != entGUIDBefore {
				t.Errorf("entity storage GUID changed on MOVE: before=%s after=%s", entGUIDBefore, got)
			}
			attrsAfter := attributeGUIDs(t, b2, dstID, entID)
			for name, want := range attrsBefore {
				got, ok := attrsAfter[name]
				if !ok {
					t.Errorf("attribute %s is missing after the move", name)
					continue
				}
				if got != want {
					t.Errorf("attribute %s storage GUID changed on MOVE: before=%s after=%s", name, want, got)
				}
			}

			assocDM := dstID
			if tc.assocInSource {
				assocDM = srcID
			}
			got := crossAssocGUID(t, b2, assocDM, assocName)
			if got == "" {
				t.Fatalf("cross-association %s not found in the expected domain model", assocName)
			}
			if got != assocGUIDBefore {
				t.Errorf("cross-association %s storage GUID changed on MOVE: before=%s after=%s",
					assocName, assocGUIDBefore, got)
			}
		})
	}
}

// associationSubject returns the first loadable domain model holding a regular
// association, with that association's name, stored GUID, and its child (TO) and
// parent (FROM) entity ids.
func associationSubject(t *testing.T, b *Backend) (dmID model.ID, moduleName, assocName, assocGUID string, childID, parentID model.ID) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		gdm, err := b.loadDomainModelGen(d.ID)
		if err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		for _, el := range gdm.AssociationsItems() {
			a, ok := el.(*genDm.Association)
			if !ok || a.Raw() == nil {
				continue
			}
			return d.ID, mod.Name, a.Name(), rawKeyHex(t, a.Raw(), "GUID"),
				model.ID(a.ChildRefID()), model.ID(a.ParentRefID())
		}
	}
	t.Fatal("no loadable domain model with a regular association in the fixture")
	return
}

// otherDomainModel returns a loadable domain model that is not exclude.
func otherDomainModel(t *testing.T, b *Backend, exclude model.ID) (model.ID, string) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID == exclude {
			continue
		}
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		return d.ID, mod.Name
	}
	return "", ""
}

func entityByID(t *testing.T, b *Backend, dmID, entID model.ID) *domainmodel.Entity {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID != dmID {
			continue
		}
		for _, e := range d.Entities {
			if e.ID == entID {
				return e
			}
		}
	}
	t.Fatalf("entity %s not found in domain model %s", entID, dmID)
	return nil
}

func findEntityByName(t *testing.T, b *Backend, dmID model.ID, name string) *domainmodel.Entity {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID != dmID {
			continue
		}
		for _, e := range d.Entities {
			if e.Name == name {
				return e
			}
		}
	}
	return nil
}

func entityGUID(t *testing.T, b *Backend, dmID, entID model.ID) string {
	return entityRawKey(t, b, dmID, entID, "GUID")
}

func entityRawKey(t *testing.T, b *Backend, dmID, entID model.ID, key string) string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	ge := findGenEntity(gdm, entID)
	if ge == nil {
		return ""
	}
	return rawKeyHex(t, ge.Raw(), key)
}

func crossAssocGUID(t *testing.T, b *Backend, dmID model.ID, name string) string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	for _, el := range gdm.CrossAssociationsItems() {
		ca, ok := el.(*genDm.CrossAssociation)
		if !ok || ca.Name() != name {
			continue
		}
		return rawKeyHex(t, ca.Raw(), "GUID")
	}
	return ""
}
