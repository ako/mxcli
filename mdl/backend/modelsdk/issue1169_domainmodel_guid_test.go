// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue1169_UpdateDomainModelPreservesStorageGUIDs guards ako/mxcli#1169:
// UpdateDomainModel must not re-mint any storage GUID in the unit it rewrites.
//
// It removes and rebuilds the WHOLE Entities and Associations lists, so every
// element arrived raw==nil and the codec's EmitGUID default wrote GUID = $ID —
// entities, their attributes and indexes, and every association, including the
// ones the statement never named. #657 fixed this for the ALTER target entity and
// #1119 for that entity's children; this path got neither, and it is the wider of
// the two, because the blast radius is the unit rather than one element.
//
// Its doc comment claimed it preserved "each element's identity". That is true of
// the $ID and false of the GUID, which is the identity that matters to the
// database: the runtime keys mendixsystem$entity.id and mendixsystem$attribute.id
// on it. The reporter measured 282 moved GUIDs in one module and a runtime crash
// (`getTableName() because "table" is null`).
//
// EVERY CASE HERE IS A DIFFERENT MDL STATEMENT, and that is the point — the title
// says ALTER ASSOCIATION, but the path is shared:
//
//	RenameEntity            RENAME ENTITY
//	RenameAssociation       RENAME ASSOCIATION
//	SetAssociationComment   ALTER ASSOCIATION ... SET COMMENT
//	SetAssociationOwner     ALTER ASSOCIATION ... SET OWNER
//	NoChange                CREATE OR MODIFY ASSOCIATION re-run
//
// RENAME ENTITY is the most expensive of them: the entity name is the table name,
// and per the #503 measurement a re-minted GUID makes the runtime DROP the old
// table and create an empty one instead of renaming it — a whole table, where an
// ALTER loses a column.
//
// CONTROL. Against the unfixed code every case fails, but on the refusal rather
// than the GUID assertion: the #1119 write guard (canon.StorageGUIDError) pairs by
// $ID within the unit, which is exactly this shape, so it catches the corruption
// before it reaches disk and UpdateDomainModel returns an error. That is the
// symptom on this branch; upstream, with no guard, the GUIDs move. The GUID
// assertions below are therefore both the primary statement of intent and the
// backstop for anyone who opts a write path out of the guard.
func TestIssue1169_UpdateDomainModelPreservesStorageGUIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		// mutate applies one statement's worth of change to the semantic model and
		// returns a check that the change actually reached disk. A fix that simply
		// stopped writing would pass every GUID assertion here.
		mutate func(t *testing.T, dm *domainmodel.DomainModel) func(t *testing.T, after *domainmodel.DomainModel)
	}{
		{
			name: "RenameEntity",
			mutate: func(t *testing.T, dm *domainmodel.DomainModel) func(*testing.T, *domainmodel.DomainModel) {
				ent := firstEntityWithAttributes(t, dm)
				id, want := ent.ID, "Issue1169Renamed"+ent.Name
				ent.Name = want
				return func(t *testing.T, after *domainmodel.DomainModel) {
					for _, e := range after.Entities {
						if e.ID == id && e.Name != want {
							t.Errorf("entity name not persisted: got %q, want %q", e.Name, want)
						}
					}
				}
			},
		},
		{
			name: "RenameAssociation",
			mutate: func(t *testing.T, dm *domainmodel.DomainModel) func(*testing.T, *domainmodel.DomainModel) {
				a := dm.Associations[0]
				id, want := a.ID, "Issue1169Renamed_"+a.Name
				a.Name = want
				return func(t *testing.T, after *domainmodel.DomainModel) {
					for _, x := range after.Associations {
						if x.ID == id && x.Name != want {
							t.Errorf("association name not persisted: got %q, want %q", x.Name, want)
						}
					}
				}
			},
		},
		{
			name: "SetAssociationComment",
			mutate: func(t *testing.T, dm *domainmodel.DomainModel) func(*testing.T, *domainmodel.DomainModel) {
				a := dm.Associations[0]
				id, want := a.ID, "issue 1169"
				a.Documentation = want
				return func(t *testing.T, after *domainmodel.DomainModel) {
					for _, x := range after.Associations {
						if x.ID == id && x.Documentation != want {
							t.Errorf("association documentation not persisted: got %q, want %q", x.Documentation, want)
						}
					}
				}
			},
		},
		{
			name: "SetAssociationOwner",
			mutate: func(t *testing.T, dm *domainmodel.DomainModel) func(*testing.T, *domainmodel.DomainModel) {
				a := dm.Associations[0]
				want := domainmodel.AssociationOwnerBoth
				if a.Owner == domainmodel.AssociationOwnerBoth {
					want = domainmodel.AssociationOwnerDefault
				}
				id := a.ID
				a.Owner = want
				return func(t *testing.T, after *domainmodel.DomainModel) {
					for _, x := range after.Associations {
						if x.ID == id && x.Owner != want {
							t.Errorf("association owner not persisted: got %q, want %q", x.Owner, want)
						}
					}
				}
			},
		},
		{
			// CREATE OR MODIFY ASSOCIATION re-run against an unchanged project. The
			// CLI's own help promises "preserves UUID. Safe to re-run" — true of the
			// $ID and, before this fix, false of the GUID. Nothing lands (ADR-0008
			// elides the write), so there is nothing to assert as persisted; what
			// matters is that it is neither refused nor destructive.
			name: "NoChange",
			mutate: func(t *testing.T, dm *domainmodel.DomainModel) func(*testing.T, *domainmodel.DomainModel) {
				return nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}

			dm := domainModelWithAssociation(t, b)
			before := domainModelGUIDs(t, b, dm.ID)

			// Preconditions. Without GUID != $ID this cannot fail against the broken
			// code: an element mxcli created has GUID == $ID from birth, so a rebuild
			// that re-mints GUID = $ID reproduces the very same value. That is the
			// trap that voided a live-database control during #503.
			divergent := 0
			for id, rec := range before {
				if rec.guid == "" {
					t.Fatalf("%s has no GUID; cannot verify preservation", rec.label)
				}
				if rec.guid != id {
					divergent++
				}
			}
			if divergent < len(before) {
				t.Fatalf("%d of %d fixture elements have GUID == $ID; all must differ to detect #1169",
					len(before)-divergent, len(before))
			}
			if divergent < 3 {
				t.Fatalf("only %d GUID-bearing elements in the fixture unit; need >= 3", divergent)
			}

			persisted := tc.mutate(t, dm)
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
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

			after := domainModelGUIDs(t, b2, dm.ID)

			moved, gone := 0, 0
			for id, want := range before {
				got, ok := after[id]
				if !ok {
					gone++
					if gone <= 3 {
						t.Errorf("%s is gone from the unit after the write", want.label)
					}
					continue
				}
				if got.guid != want.guid {
					moved++
					if moved <= 5 {
						t.Errorf("%s: storage GUID changed %s -> %s", want.label, want.guid, got.guid)
					}
				}
			}
			if moved > 5 {
				t.Errorf("... and %d more elements whose storage GUID changed", moved-5)
			}
			if gone > 3 {
				t.Errorf("... and %d more elements gone from the unit", gone-3)
			}

			if persisted != nil {
				dmAfter := reloadDomainModel(t, b2, dm.ID)
				persisted(t, dmAfter)
			}
		})
	}
}

// guidRec is one GUID-bearing element: its human label, for a failure message
// that says WHICH element moved, and its stored GUID.
type guidRec struct {
	label string
	guid  string
}

// domainModelGUIDs censuses every GUID-bearing element in a domain model unit,
// keyed by element $ID.
//
// The key is the $ID and not the name, deliberately: a RENAME case keyed on name
// reads as "the old element vanished and a new one appeared", which looks like
// four changes and one addition rather than the zero it is. That false reading
// cost a measurement during this investigation.
func domainModelGUIDs(t *testing.T, b *Backend, dmID model.ID) map[string]guidRec {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	out := map[string]guidRec{}
	for _, el := range gdm.EntitiesItems() {
		ge, ok := el.(*genDm.Entity)
		if !ok {
			continue
		}
		out[string(ge.ID())] = guidRec{fmt.Sprintf("entity %s", ge.Name()), rawKeyHex(t, ge.Raw(), "GUID")}
		for _, ael := range ge.AttributesItems() {
			if a, ok := ael.(*genDm.Attribute); ok {
				out[string(a.ID())] = guidRec{
					fmt.Sprintf("attribute %s.%s", ge.Name(), a.Name()),
					rawKeyHex(t, a.Raw(), "GUID"),
				}
			}
		}
		for i, iel := range ge.IndexesItems() {
			if idx, ok := iel.(*genDm.Index); ok {
				out[string(idx.ID())] = guidRec{
					fmt.Sprintf("index %s[%d]", ge.Name(), i),
					rawKeyHex(t, idx.Raw(), "GUID"),
				}
			}
		}
	}
	for _, el := range gdm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok {
			out[string(a.ID())] = guidRec{fmt.Sprintf("association %s", a.Name()), rawKeyHex(t, a.Raw(), "GUID")}
		}
	}
	for _, el := range gdm.CrossAssociationsItems() {
		if a, ok := el.(*genDm.CrossAssociation); ok {
			out[string(a.ID())] = guidRec{fmt.Sprintf("cross-association %s", a.Name()), rawKeyHex(t, a.Raw(), "GUID")}
		}
	}
	return out
}

// domainModelWithAssociation returns the first loadable domain model that holds
// both an association and an entity with attributes.
func domainModelWithAssociation(t *testing.T, b *Backend) *domainmodel.DomainModel {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue // the System module's unit is not on disk in the fixture
		}
		if len(d.Associations) == 0 {
			continue
		}
		for _, e := range d.Entities {
			if len(e.Attributes) > 0 {
				return d
			}
		}
	}
	t.Fatal("no loadable domain model in the fixture holds an association and an entity with attributes")
	return nil
}

func firstEntityWithAttributes(t *testing.T, dm *domainmodel.DomainModel) *domainmodel.Entity {
	t.Helper()
	for _, e := range dm.Entities {
		if len(e.Attributes) > 0 {
			return e
		}
	}
	t.Fatal("domain model has no entity with attributes")
	return nil
}

func reloadDomainModel(t *testing.T, b *Backend, dmID model.ID) *domainmodel.DomainModel {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID == dmID {
			return d
		}
	}
	t.Fatalf("domain model %s not found after the write", dmID)
	return nil
}
