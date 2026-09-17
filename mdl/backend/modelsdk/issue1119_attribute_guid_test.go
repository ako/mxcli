// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue1119_AlterPreservesAttributeGUIDs guards GitHub issue #1119: an ALTER
// of an existing entity must not change any attribute's storage GUID.
//
// #657 closed this for the entity element. Its children were left behind:
// entityToGen rebuilds every attribute from the semantic model, so each arrived
// raw==nil and the codec's EmitGUID default wrote GUID = $ID. The runtime keys
// mendixsystem$attribute.id on that GUID, so on the next deploy the synchroniser
// treats every attribute as deleted-and-re-added and drops its column — reported
// from production as 28 attributes of 607 rows emptied by a single MDL edit.
//
// Every ALTER ENTITY form routes through Backend.UpdateEntity (cmd_entities.go),
// so exercising that covers all of them — including SET DOCUMENTATION, which
// touches no attribute at all and destroyed every GUID anyway.
func TestIssue1119_AlterPreservesAttributeGUIDs(t *testing.T) {
	cases := []struct {
		name string
		// mutate applies one ALTER form. newAttr names an attribute the mutation
		// creates, and renamedFrom/renamedTo the pair a rename produces.
		mutate      func(*domainmodel.Entity)
		newAttr     string
		renamedTo   string
		renamedFrom int // index into the entity's attributes, before mutation
	}{
		{
			name:   "SetDocumentation",
			mutate: func(e *domainmodel.Entity) { e.Documentation = "issue 1119" },
		},
		{
			name: "AddAttribute",
			mutate: func(e *domainmodel.Entity) {
				e.Attributes = append(e.Attributes, &domainmodel.Attribute{
					Name: "Issue1119Added",
					Type: &domainmodel.StringAttributeType{Length: 20},
				})
			},
			newAttr: "Issue1119Added",
		},
		{
			name:        "RenameAttribute",
			mutate:      func(e *domainmodel.Entity) { e.Attributes[0].Name = "Issue1119Renamed" },
			renamedTo:   "Issue1119Renamed",
			renamedFrom: 0,
		},
		{
			name:   "DropAttribute",
			mutate: func(e *domainmodel.Entity) { e.Attributes = e.Attributes[1:] },
		},
		{
			name: "ModifyAttribute",
			mutate: func(e *domainmodel.Entity) {
				e.Attributes[1].Type = &domainmodel.StringAttributeType{Length: 999}
			},
		},
		{
			name:   "RenameEntity",
			mutate: func(e *domainmodel.Entity) { e.Name = "Issue1119Renamed" + e.Name },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}

			dmID, ent := richestEntity(t, b)
			before := attributeGUIDs(t, b, dmID, ent.ID)
			if len(before) < 3 {
				t.Fatalf("fixture entity %s has %d attributes; need >= 3", ent.Name, len(before))
			}
			// Without GUID != $ID the test cannot detect the conflation at all —
			// it would pass against the broken code. Same precondition as #657's.
			ids := attributeIDs(t, b, dmID, ent.ID)
			for name, guid := range before {
				if guid == "" {
					t.Fatalf("fixture attribute %s has no GUID; cannot verify preservation", name)
				}
				if guid == ids[name] {
					t.Fatalf("fixture attribute %s has GUID == $ID; need them to differ to detect #1119", name)
				}
			}
			originalName := ent.Attributes[tc.renamedFrom].Name

			tc.mutate(ent)
			if err := b.UpdateEntity(dmID, ent); err != nil {
				t.Fatalf("UpdateEntity: %v", err)
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			// Reopen: GUID preservation only counts if it reached disk.
			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })
			after := attributeGUIDs(t, b2, dmID, ent.ID)

			// Every attribute that survived under its own name keeps its GUID.
			changed := 0
			for name, want := range before {
				got, ok := after[name]
				if !ok {
					continue // dropped or renamed; handled below
				}
				if got != want {
					changed++
					if changed <= 3 {
						t.Errorf("attribute %s: storage GUID changed %s -> %s", name, want, got)
					}
				}
			}
			if changed > 3 {
				t.Errorf("... and %d more attributes whose storage GUID changed", changed-3)
			}

			// A rename must carry the GUID FORWARD: Studio Pro renames the column
			// and keeps the data. Minting a fresh one here loses it just as surely
			// as changing an untouched attribute's.
			if tc.renamedTo != "" {
				if got, want := after[tc.renamedTo], before[originalName]; got != want {
					t.Errorf("rename %s -> %s did not carry the storage GUID: %s -> %s",
						originalName, tc.renamedTo, want, got)
				}
			}

			// And a genuinely new attribute must get a GUID of its own — not an
			// empty one, and not one inherited from an existing or dropped
			// attribute, which would make the runtime adopt that column.
			if tc.newAttr != "" {
				got := after[tc.newAttr]
				if got == "" {
					t.Errorf("new attribute %s has no storage GUID", tc.newAttr)
				}
				for name, old := range before {
					if old == got {
						t.Errorf("new attribute %s inherited %s's storage GUID %s", tc.newAttr, name, got)
					}
				}
			}
		})
	}
}

// TestIssue1119_AlterPreservesAssociationGUIDs is the control for the reporter's
// own negative result: a sibling association in the same unit was already safe,
// because the entities-list rebuild passes it through as raw. It is here so a
// future change that starts rebuilding DM-level children is caught with the
// attribute case rather than after it.
func TestIssue1119_AlterPreservesAssociationGUIDs(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Find a domain model that holds both an association and an entity to alter.
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	var (
		dmID model.ID
		ent  *domainmodel.Entity
	)
	for _, d := range dms {
		gdm, err := b.loadDomainModelGen(d.ID)
		if err != nil || len(gdm.AssociationsItems()) == 0 {
			continue
		}
		for _, e := range d.Entities {
			if len(e.Attributes) > 0 {
				dmID, ent = d.ID, e
				break
			}
		}
		if ent != nil {
			break
		}
	}
	if ent == nil {
		t.Skip("no domain model in the fixture holds both an association and an entity with attributes")
	}

	before := associationGUIDs(t, b, dmID)
	if len(before) == 0 {
		t.Fatal("no associations read back; the control cannot prove anything")
	}

	ent.Documentation = "issue 1119 association control"
	if err := b.UpdateEntity(dmID, ent); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	after := associationGUIDs(t, b2, dmID)
	for name, want := range before {
		if got := after[name]; got != want {
			t.Errorf("association %s: storage GUID changed %s -> %s", name, want, got)
		}
	}
}

// richestEntity returns the loadable domain model and the entity with the most
// attributes in the fixture — the most sensitive target for a GUID check.
func richestEntity(t *testing.T, b *Backend) (model.ID, *domainmodel.Entity) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	var (
		dmID model.ID
		best *domainmodel.Entity
	)
	for _, d := range dms {
		// The System module's unit is not on disk in the fixture; skip what we
		// cannot read rather than failing on it.
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		for _, e := range d.Entities {
			if best == nil || len(e.Attributes) > len(best.Attributes) {
				dmID, best = d.ID, e
			}
		}
	}
	if best == nil {
		t.Fatal("no loadable domain model with entities in the fixture")
	}
	return dmID, best
}

// attributeGUIDs maps attribute name -> storage GUID, read from the raw BSON.
// The GUID is not surfaced by the semantic reader, which is why #657's finding
// says to assert on raw bytes rather than on DESCRIBE.
func attributeGUIDs(t *testing.T, b *Backend, dmID, entID model.ID) map[string]string {
	return attributeRawKey(t, b, dmID, entID, "GUID")
}

// attributeIDs maps attribute name -> element $ID, for the GUID != $ID precondition.
func attributeIDs(t *testing.T, b *Backend, dmID, entID model.ID) map[string]string {
	return attributeRawKey(t, b, dmID, entID, "$ID")
}

func attributeRawKey(t *testing.T, b *Backend, dmID, entID model.ID, key string) map[string]string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	ge := findGenEntity(gdm, entID)
	if ge == nil {
		t.Fatalf("gen entity %s not found", entID)
	}
	out := map[string]string{}
	for _, el := range ge.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok {
			out[a.Name()] = rawKeyHex(t, a.Raw(), key)
		}
	}
	return out
}

func associationGUIDs(t *testing.T, b *Backend, dmID model.ID) map[string]string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	out := map[string]string{}
	for _, el := range gdm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok {
			out[a.Name()] = rawKeyHex(t, a.Raw(), "GUID")
		}
	}
	return out
}
