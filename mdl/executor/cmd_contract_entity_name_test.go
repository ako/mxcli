// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1432: "Bulk external-entity creation names each entity after
// the OData entity set (`Parts`), while Studio Pro names it after the entity type
// (`Part`). A script that refers to the Studio Pro name then fails."
const partsContract = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Demo" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Part">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Int64" Nullable="false"/>
        <Property Name="Name" Type="Edm.String"/>
        <NavigationProperty Name="Vendor" Type="Demo.Supplier"/>
      </EntityType>
      <EntityType Name="Supplier">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Int64" Nullable="false"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Parts" EntityType="Demo.Part"/>
        <EntitySet Name="Suppliers" EntityType="Demo.Supplier"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

type partsImport struct {
	created map[string]*domainmodel.Entity
	updated map[string]*domainmodel.Entity
	assocs  []*domainmodel.Association
	out     string
}

// importParts runs CREATE OR MODIFY EXTERNAL ENTITIES FROM over partsContract
// into a module that already holds the given entities.
func importParts(t *testing.T, existing ...*domainmodel.Entity) partsImport {
	t.Helper()
	mod := mkModule("ModA")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "ClientA",
		MetadataUrl:  "http://localhost:18765/metadata.xml",
		ODataVersion: "4.0",
		Metadata:     partsContract,
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	dm := &domainmodel.DomainModel{Entities: existing}
	dm.ID = nextID("dm")

	r := partsImport{created: map[string]*domainmodel.Entity{}, updated: map[string]*domainmodel.Entity{}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			r.created[e.Name] = e
			dm.Entities = append(dm.Entities, e)
			return nil
		},
		UpdateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			r.updated[e.Name] = e
			return nil
		},
		CreateAssociationFunc: func(_ model.ID, a *domainmodel.Association) error {
			r.assocs = append(r.assocs, a)
			dm.Associations = append(dm.Associations, a)
			return nil
		},
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: 11, MinorVersion: 13, ProductVersion: "11.13.0"}
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntitiesStmt{
		ServiceRef:     ast.QualifiedName{Module: "ModA", Name: "ClientA"},
		TargetModule:   "ModA",
		CreateOrModify: true,
	}
	assertNoError(t, createExternalEntities(ctx, stmt))
	r.out = buf.String()
	return r
}

func TestCreateExternalEntities_NamesEntityAfterTheEntityType(t *testing.T) {
	r := importParts(t)
	if _, ok := r.created["Parts"]; ok {
		t.Errorf("entity named after the entity set (ModA.Parts); Studio Pro names it after the type (ModA.Part)")
	}
	part := r.created["Part"]
	if part == nil {
		t.Fatalf("ModA.Part not created; got %v", entityNamesOf(r.created))
	}
	// The entity set is still what the runtime queries.
	if part.RemoteEntitySet != "Parts" || part.RemoteEntityName != "Part" {
		t.Errorf("RemoteEntitySet=%q RemoteEntityName=%q, want Parts / Part", part.RemoteEntitySet, part.RemoteEntityName)
	}
	if r.created["Supplier"] == nil {
		t.Errorf("ModA.Supplier not created; got %v", entityNamesOf(r.created))
	}
	// The navigation pass must find the type-named entities, or the association
	// silently disappears.
	if len(r.assocs) != 1 || r.assocs[0].Name != "Vendor" ||
		r.assocs[0].ParentID != part.ID || r.assocs[0].ChildID != r.created["Supplier"].ID {
		t.Errorf("want one association Vendor from Part to Supplier, got %d", len(r.assocs))
	}
}

// A module imported by an older mxcli holds the entity under its entity-set
// name. Re-importing must update that entity in place — a type-named twin
// beside it would map the same entity set twice, and renaming it would break
// every page and microflow written against the old name.
func TestCreateExternalEntities_ReimportKeepsAnEntitySetNamedEntity(t *testing.T) {
	legacy := &domainmodel.Entity{
		Name:              "Parts",
		Source:            "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "ModA.ClientA",
		RemoteEntitySet:   "Parts",
		RemoteEntityName:  "Part",
		Persistable:       true,
	}
	legacy.ID = nextID("ent")

	r := importParts(t, legacy)
	if r.created["Part"] != nil {
		t.Errorf("re-import created ModA.Part beside the existing ModA.Parts — the entity set is now mapped twice")
	}
	if r.updated["Parts"] == nil {
		t.Errorf("the existing ModA.Parts was not updated; updated=%v created=%v",
			entityNamesOf(r.updated), entityNamesOf(r.created))
	}
	if !strings.Contains(r.out, "1 created, 1 updated") {
		t.Errorf("want Supplier created and Parts updated:\n%s", r.out)
	}
	// The Vendor association must still resolve its FROM end to the legacy entity.
	if len(r.assocs) != 1 || r.assocs[0].ParentID != legacy.ID {
		t.Errorf("Vendor association does not start at the existing ModA.Parts")
	}
}

// The same entity set from a DIFFERENT service is not the same entity.
func TestCreateExternalEntities_ReimportIgnoresAnotherServicesEntitySet(t *testing.T) {
	other := &domainmodel.Entity{
		Name:              "Parts",
		Source:            "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "ModB.OtherClient",
		RemoteEntitySet:   "Parts",
		Persistable:       true,
	}
	other.ID = nextID("ent")

	r := importParts(t, other)
	if r.created["Part"] == nil {
		t.Errorf("ModA.Part not created; the other service's Parts was taken for it")
	}
	if r.updated["Parts"] != nil {
		t.Errorf("another service's entity was overwritten")
	}
}
