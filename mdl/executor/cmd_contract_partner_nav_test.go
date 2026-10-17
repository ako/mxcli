// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1231: importing an OData entity with *-to-* navigation
// properties "auto-creates ReferenceSet associations … These associations are
// invalid for external entities and cause CE6618 at mxbuild time", and "check
// and lint are both silent".
//
// Measured on 11.12.1, the line is the navigation's Partner, not its
// multiplicity: an unpartnered collection builds clean, and every PARTNERED end
// imported as its own association is CE6618. A one-to-many pair builds as ONE
// association naming both ends; a many-to-many or one-to-one pair builds in no
// form at all.

func navType(name string, navs ...*types.EdmNavigationProperty) *types.EdmEntityType {
	return &types.EdmEntityType{Name: name, KeyProperties: []string{"Id"}, NavigationProperties: navs}
}

func nav(name, typ, partner string) *types.EdmNavigationProperty {
	return &types.EdmNavigationProperty{Name: name, Type: typ, Partner: partner}
}

type navRun struct {
	dm      *domainmodel.DomainModel
	ents    map[string]*domainmodel.Entity
	created []*domainmodel.Association
	doc     *types.EdmxDocument
	byQN    map[string]*types.EdmEntityType
	esMap   map[string]string
}

// newNavRun builds a domain model holding one persistable external entity per
// type, named after the type, each backed by its own entity set.
func newNavRun(ets ...*types.EdmEntityType) *navRun {
	r := &navRun{ents: map[string]*domainmodel.Entity{}, byQN: map[string]*types.EdmEntityType{}, esMap: map[string]string{}}
	r.dm = &domainmodel.DomainModel{}
	r.dm.ID = model.ID("dm-nav")
	r.doc = &types.EdmxDocument{Schemas: []*types.EdmSchema{{Namespace: "Demo", EntityTypes: ets}}}
	for _, et := range ets {
		qn := "Demo." + et.Name
		e := &domainmodel.Entity{Name: et.Name, Persistable: true}
		e.ID = model.ID("ent-" + et.Name)
		r.dm.Entities = append(r.dm.Entities, e)
		r.ents[et.Name] = e
		r.byQN[qn] = et
		r.esMap[qn] = et.Name + "s"
		r.doc.EntitySets = append(r.doc.EntitySets, &types.EdmEntitySet{Name: et.Name + "s", EntityType: qn})
	}
	return r
}

func (r *navRun) run(t *testing.T) (int, string) {
	t.Helper()
	mb := &mock.MockBackend{
		CreateAssociationFunc: func(_ model.ID, a *domainmodel.Association) error {
			r.created = append(r.created, a)
			r.dm.Associations = append(r.dm.Associations, a)
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	n := createNavigationAssociations(ctx, r.dm, r.doc, r.byQN, r.esMap, "Ext.Svc")
	return n, buf.String()
}

func TestPartneredOneToManyIsOneAssociation(t *testing.T) {
	r := newNavRun(
		navType("Buffer", nav("Lines", "Collection(Demo.Line)", "Buffer")),
		navType("Line", nav("Buffer", "Demo.Buffer", "Lines")),
	)
	if n, out := r.run(t); n != 1 {
		t.Fatalf("created %d associations, want 1 — each end as its own association is CE6618 on both:\n%s", n, out)
	}
	a := r.created[0]
	// The single-valued end holds the reference, so it is the FROM entity.
	if a.ParentID != r.ents["Line"].ID || a.ChildID != r.ents["Buffer"].ID {
		t.Errorf("association runs %s -> %s, want Line -> Buffer", a.ParentID, a.ChildID)
	}
	if a.Type != domainmodel.AssociationTypeReference {
		t.Errorf("Type = %s, want Reference", a.Type)
	}
	if a.RemoteParentNavigationProperty != "Buffer" || a.RemoteChildNavigationProperty != "Lines" {
		t.Errorf("navigation properties %q / %q, want Buffer / Lines",
			a.RemoteParentNavigationProperty, a.RemoteChildNavigationProperty)
	}
	if a.Navigability2 != "BothDirections" {
		t.Errorf("Navigability2 = %q, want BothDirections", a.Navigability2)
	}

	// A re-import must recognise the pair from either end.
	if n, out := r.run(t); n != 0 {
		t.Errorf("re-import created %d more associations, want 0:\n%s", n, out)
	}
}

func TestPartneredManyToManyAndOneToOneAreNotImported(t *testing.T) {
	r := newNavRun(
		navType("Buffer",
			nav("CapacityTypes", "Collection(Demo.CapacityType)", "Buffers"),
			nav("Twin", "Demo.Twin", "TwinBuffer"),
			// CONTROL: an unpartnered collection builds clean and must stay.
			nav("Peers", "Collection(Demo.Buffer)", "")),
		navType("CapacityType", nav("Buffers", "Collection(Demo.Buffer)", "CapacityTypes")),
		navType("Twin", nav("TwinBuffer", "Demo.Buffer", "Twin")),
	)
	n, out := r.run(t)
	if n != 1 || r.created[0].Name != "Peers" {
		names := []string{}
		for _, a := range r.created {
			names = append(names, a.Name)
		}
		t.Fatalf("created %v, want only Peers — a partnered many-to-many or one-to-one end is CE6618", names)
	}
	if r.created[0].Type != domainmodel.AssociationTypeReferenceSet || r.created[0].RemoteChildNavigationProperty != "" {
		t.Errorf("the unpartnered Peers association changed shape")
	}
	for _, want := range []string{
		"Buffer.CapacityTypes / CapacityType.Buffers (many-to-many)",
		"Buffer.Twin / Twin.TwinBuffer (one-to-one)",
		"CE6618",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not report %q:\n%s", want, out)
		}
	}
	// Once per pair, not once per end.
	if c := strings.Count(out, "CapacityTypes / "); c != 1 {
		t.Errorf("the many-to-many pair is reported %d times, want 1:\n%s", c, out)
	}
}

// A module imported before this fix already holds the CE6618 associations.
// They are not ours to delete silently, but the import must name them.
func TestReimportNamesAssociationsThatMapAPartneredEndSeparately(t *testing.T) {
	r := newNavRun(
		navType("Buffer",
			nav("CapacityTypes", "Collection(Demo.CapacityType)", "Buffers"),
			nav("Lines", "Collection(Demo.Line)", "Buffer")),
		navType("CapacityType", nav("Buffers", "Collection(Demo.Buffer)", "CapacityTypes")),
		navType("Line", nav("Buffer", "Demo.Buffer", "Lines")),
	)
	old := func(name, from, to, navName string) {
		a := &domainmodel.Association{Name: name, ParentID: r.ents[from].ID, ChildID: r.ents[to].ID,
			Source: "Rest$ODataRemoteAssociationSource", RemoteParentNavigationProperty: navName}
		a.ID = model.ID("old-" + name)
		r.dm.Associations = append(r.dm.Associations, a)
	}
	old("CapacityTypes", "Buffer", "CapacityType", "CapacityTypes")
	old("Buffers", "CapacityType", "Buffer", "Buffers")
	old("Lines", "Buffer", "Line", "Lines")
	old("Buffer_2", "Line", "Buffer", "Buffer")

	n, out := r.run(t)
	if n != 0 {
		t.Errorf("created %d associations beside the old ones, want 0", n)
	}
	for _, name := range []string{"CapacityTypes", "Buffers", "Lines", "Buffer_2"} {
		if !strings.Contains(out, "drop association "+name+";") {
			t.Errorf("output does not name %s for dropping:\n%s", name, out)
		}
	}
}
