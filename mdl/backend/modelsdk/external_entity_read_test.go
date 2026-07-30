// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#782: an external entity's `allow_create_change_locally` flag
// would not stick. The write path handled it; the *read* did not. entityFromGen
// recognised only DomainModels$OqlViewEntitySource, so an OData external entity
// came back with an empty Source and every remote field zeroed. Setting the flag
// then wrote it onto a model that no longer knew it was external, and the value
// was lost — as were describe, and any other read-modify-write on such an entity.
//
// The legacy engine (sdk/mpr/parser_domainmodel.go) parsed all three OData source
// flavours, so this was a modelsdk-engine gap, not a missing feature.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// externalEntityFixture creates an OData external entity through the write path,
// then reads the domain model back with a fresh connection. Round-tripping through
// disk is the point: the read is what #782 broke.
func externalEntityFixture(t *testing.T, mutate func(*domainmodel.Entity)) (proj string, moduleID model.ID) {
	t.Helper()
	proj = copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v, %v", mod, err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel: %v, %v", dm, err)
	}

	ent := &domainmodel.Entity{
		Name:                "Products",
		Persistable:         true,
		Source:              "Rest$ODataRemoteEntitySource",
		RemoteServiceName:   "MyFirstModule.Sample",
		RemoteEntitySet:     "Products",
		RemoteEntityName:    "Product",
		Countable:           true,
		Creatable:           true,
		Deletable:           true,
		SkipSupported:       true,
		TopSupported:        true,
		CreateChangeLocally: true,
		RemoteKeyParts: []*domainmodel.RemoteKeyPart{{
			Name:       "ProductID",
			RemoteName: "ProductID",
			RemoteType: "Edm.Int32",
			Type:       &domainmodel.IntegerAttributeType{},
		}},
		Attributes: []*domainmodel.Attribute{
			{Name: "ProductName", Type: &domainmodel.StringAttributeType{}},
		},
	}
	if mutate != nil {
		mutate(ent)
	}
	if err := b.CreateEntity(dm.ID, ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	return proj, mod.ID
}

func readEntity(t *testing.T, proj string, moduleID model.ID, name string) *domainmodel.Entity {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	dm, err := b.GetDomainModel(moduleID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel: %v, %v", dm, err)
	}
	for _, e := range dm.Entities {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entity %s not found after write", name)
	return nil
}

// TestExternalEntity_RemoteSourceRoundTrip is the regression test: every field of
// a Rest$ODataRemoteEntitySource must survive a write→read cycle.
func TestExternalEntity_RemoteSourceRoundTrip(t *testing.T) {
	proj, modID := externalEntityFixture(t, nil)
	got := readEntity(t, proj, modID, "Products")

	if got.Source != "Rest$ODataRemoteEntitySource" {
		t.Fatalf("Source = %q, want Rest$ODataRemoteEntitySource — the entity does not read back as external", got.Source)
	}
	if !got.CreateChangeLocally {
		t.Error("CreateChangeLocally = false, want true (#782)")
	}
	if got.RemoteServiceName != "MyFirstModule.Sample" {
		t.Errorf("RemoteServiceName = %q", got.RemoteServiceName)
	}
	if got.RemoteEntitySet != "Products" {
		t.Errorf("RemoteEntitySet = %q", got.RemoteEntitySet)
	}
	if got.RemoteEntityName != "Product" {
		t.Errorf("RemoteEntityName = %q", got.RemoteEntityName)
	}
	if !got.Countable || !got.Creatable || !got.Deletable || !got.SkipSupported || !got.TopSupported {
		t.Errorf("capability flags lost: countable=%v creatable=%v deletable=%v skip=%v top=%v",
			got.Countable, got.Creatable, got.Deletable, got.SkipSupported, got.TopSupported)
	}
	if got.SourceObjectID == "" {
		t.Error("SourceObjectID empty — a read-modify-write cannot preserve the source element")
	}
	if len(got.RemoteKeyParts) != 1 {
		t.Fatalf("RemoteKeyParts = %+v, want 1 part", got.RemoteKeyParts)
	}
	kp := got.RemoteKeyParts[0]
	if kp.Name != "ProductID" || kp.RemoteName != "ProductID" || kp.RemoteType != "Edm.Int32" {
		t.Errorf("key part = %+v", kp)
	}
	if _, ok := kp.Type.(*domainmodel.IntegerAttributeType); !ok {
		t.Errorf("key part type = %T, want *IntegerAttributeType", kp.Type)
	}
}

// TestExternalEntity_FlagSurvivesReadModifyWrite reproduces the reported flow:
// read the entity, flip the flag (what ALTER ENTITY … SET
// ALLOW_CREATE_CHANGE_LOCALLY does), write it back, read again.
func TestExternalEntity_FlagSurvivesReadModifyWrite(t *testing.T) {
	proj, modID := externalEntityFixture(t, func(e *domainmodel.Entity) {
		e.CreateChangeLocally = false // imported off, as CREATE EXTERNAL ENTITIES leaves it
	})

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	dm, err := b.GetDomainModel(modID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	var ent *domainmodel.Entity
	for _, e := range dm.Entities {
		if e.Name == "Products" {
			ent = e
		}
	}
	if ent == nil {
		t.Fatal("Products not found")
	}
	ent.CreateChangeLocally = true
	if err := b.UpdateEntity(dm.ID, ent); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	got := readEntity(t, proj, modID, "Products")
	if !got.CreateChangeLocally {
		t.Error("CreateChangeLocally = false after setting it to true (#782)")
	}
	// The rest of the source must not have been damaged by the update.
	if got.Source != "Rest$ODataRemoteEntitySource" || got.RemoteEntitySet != "Products" {
		t.Errorf("source damaged by the update: Source=%q EntitySet=%q", got.Source, got.RemoteEntitySet)
	}
	if len(got.RemoteKeyParts) != 1 {
		t.Errorf("remote key lost by the update: %+v", got.RemoteKeyParts)
	}
}

// TestExternalEntity_EntityTypeSourceRoundTrip covers the second flavour: a
// derived/abstract/contained type, which has no entity set.
func TestExternalEntity_EntityTypeSourceRoundTrip(t *testing.T) {
	proj, modID := externalEntityFixture(t, func(e *domainmodel.Entity) {
		e.Name = "ProductDetail"
		e.Source = "Rest$ODataEntityTypeSource"
		e.Persistable = false
		e.IsOpen = true
		e.RemoteEntitySet = ""
	})
	got := readEntity(t, proj, modID, "ProductDetail")

	if got.Source != "Rest$ODataEntityTypeSource" {
		t.Fatalf("Source = %q, want Rest$ODataEntityTypeSource", got.Source)
	}
	if got.RemoteEntityName != "Product" {
		t.Errorf("RemoteEntityName = %q, want Product", got.RemoteEntityName)
	}
	if !got.IsOpen {
		t.Error("IsOpen = false, want true")
	}
	if got.RemoteServiceName != "MyFirstModule.Sample" {
		t.Errorf("RemoteServiceName = %q", got.RemoteServiceName)
	}
	if len(got.RemoteKeyParts) != 1 {
		t.Errorf("RemoteKeyParts = %+v, want 1 part", got.RemoteKeyParts)
	}
}

// TestExternalEntity_PrimitiveCollectionSourceRoundTrip covers the third flavour,
// the NPE generated for a Collection(Edm.*) property.
func TestExternalEntity_PrimitiveCollectionSourceRoundTrip(t *testing.T) {
	proj, modID := externalEntityFixture(t, func(e *domainmodel.Entity) {
		e.Name = "ProductTag"
		e.Source = "Rest$ODataPrimitiveCollectionEntitySource"
		e.Persistable = false
		e.RemoteEntitySet = ""
		e.RemoteKeyParts = nil
	})
	got := readEntity(t, proj, modID, "ProductTag")

	if got.Source != "Rest$ODataPrimitiveCollectionEntitySource" {
		t.Fatalf("Source = %q, want Rest$ODataPrimitiveCollectionEntitySource", got.Source)
	}
	if got.RemoteServiceName != "MyFirstModule.Sample" {
		t.Errorf("RemoteServiceName = %q", got.RemoteServiceName)
	}
}
